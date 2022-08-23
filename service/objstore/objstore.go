package objstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/atomic"
	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
)

const (
	subdirpathSeparator = "/"
)

type (
	// Objstore is a service wrapper around a object storage client
	Objstore interface {
		GetBucket(name string) Bucket
		DelBucket(ctx context.Context, name string) error
	}

	// Service is an Objstore and governor.Service
	Service interface {
		governor.Service
		Objstore
	}

	getClientRes struct {
		client *minio.Client
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getClientRes
	}

	service struct {
		client     *minio.Client
		auth       minioauth
		addr       string
		sslmode    bool
		location   string
		config     governor.SecretReader
		logger     governor.Logger
		ops        chan getOp
		ready      *atomic.Bool
		hbfailed   int
		hbinterval int
		hbmaxfail  int
		done       <-chan struct{}
	}

	ctxKeyObjstore struct{}

	ctxKeyBucket struct{}
)

// getCtxObjstore returns an Objstore from the context
func getCtxObjstore(inj governor.Injector) Objstore {
	v := inj.Get(ctxKeyObjstore{})
	if v == nil {
		return nil
	}
	return v.(Objstore)
}

// setCtxObjstore sets an Objstore in the context
func setCtxObjstore(inj governor.Injector, o Objstore) {
	inj.Set(ctxKeyObjstore{}, o)
}

// GetCtxBucket returns a Bucket from the context
func GetCtxBucket(inj governor.Injector) Bucket {
	v := inj.Get(ctxKeyBucket{})
	if v == nil {
		return nil
	}
	return v.(Bucket)
}

// setCtxBucket sets a Bucket in the context
func setCtxBucket(inj governor.Injector, b Bucket) {
	inj.Set(ctxKeyBucket{}, b)
}

// NewBucketInCtx creates a new bucket from a context and sets it in the context
func NewBucketInCtx(inj governor.Injector, name string) {
	obj := getCtxObjstore(inj)
	setCtxBucket(inj, obj.GetBucket(name))
}

// New creates a new object store service instance
func New() Service {
	return &service{
		ops:      make(chan getOp),
		ready:    &atomic.Bool{},
		hbfailed: 0,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxObjstore(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "9000")
	r.SetDefault("sslmode", false)
	r.SetDefault("location", "us-east-1")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 5)
}

type (
	// ErrConn is returned on an objstore connection error
	ErrConn struct{}
	// ErrClient is returned for unknown client errors
	ErrClient struct{}
	// ErrNotFound is returned when an object is not found
	ErrNotFound struct{}
)

func (e ErrConn) Error() string {
	return "Objstore connection error"
}

func (e ErrClient) Error() string {
	return "Objstore client error"
}

func (e ErrNotFound) Error() string {
	return "Object not found"
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.sslmode = r.GetBool("sslmode")
	s.location = r.GetStr("location")
	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")

	l.Info("loaded config", map[string]string{
		"addr":       s.addr,
		"sslmode":    strconv.FormatBool(s.sslmode),
		"location":   s.location,
		"hbinterval": strconv.Itoa(s.hbinterval),
		"hbmaxfail":  strconv.Itoa(s.hbmaxfail),
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	return nil
}

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Duration(s.hbinterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.closeClient()
			return
		case <-ticker.C:
			s.handlePing(ctx)
		case op := <-s.ops:
			client, err := s.handleGetClient(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getClientRes{
				client: client,
				err:    err,
			}:
				close(op.res)
			}
		}
	}
}

func (s *service) handlePing(ctx context.Context) {
	if s.client != nil {
		_, err := s.client.ListBuckets(ctx)
		if err == nil {
			s.ready.Store(true)
			s.hbfailed = 0
			return
		}
		s.hbfailed++
		if s.hbfailed < s.hbmaxfail {
			s.logger.Warn("Failed to ping objstore", map[string]string{
				"error":      err.Error(),
				"actiontype": "objstore_ping",
				"address":    s.addr,
				"username":   s.auth.Username,
			})
			return
		}
		s.logger.Warn("Failed max pings to objstore", map[string]string{
			"error":      err.Error(),
			"actiontype": "objstore_ping",
			"address":    s.addr,
			"username":   s.auth.Username,
		})
		s.ready.Store(false)
		s.hbfailed = 0
		s.auth = minioauth{}
		s.config.InvalidateSecret("auth")
	}
	if _, err := s.handleGetClient(ctx); err != nil {
		s.logger.Error("Failed to create objstore client", map[string]string{
			"error":      err.Error(),
			"actiontype": "objstore_create_client",
		})
	}
}

type (
	minioauth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *service) handleGetClient(ctx context.Context) (*minio.Client, error) {
	var auth minioauth
	if err := s.config.GetSecret(ctx, "auth", 0, &auth); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid secret")
	}
	if auth.Username == "" {
		return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	client, err := minio.New(s.addr, &minio.Options{
		Creds:  credentials.NewStaticV4(auth.Username, auth.Password, ""),
		Secure: s.sslmode,
	})
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient{}, "Failed to create objstore client")
	}
	if _, err := client.ListBuckets(ctx); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrConn{}, "Failed to ping objstore")
	}

	s.client = client
	s.auth = auth
	s.ready.Store(false)
	s.hbfailed = 0
	s.logger.Info(fmt.Sprintf("Established connection to %s with key %s", s.addr, s.auth.Username), nil)
	return s.client, nil
}

func (s *service) closeClient() {
	s.client = nil
	s.auth = minioauth{}
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		l.Warn("Failed to stop", map[string]string{
			"error":      ctx.Err().Error(),
			"actiontype": "objstore_stop",
		})
	}
}

func (s *service) Health() error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, ErrConn{}, "Objstore service not ready")
	}
	return nil
}

func (s *service) getClient(ctx context.Context) (*minio.Client, error) {
	res := make(chan getClientRes)
	op := getOp{
		ctx: ctx,
		res: res,
	}
	select {
	case <-s.done:
		return nil, kerrors.WithMsg(nil, "Objstore service shutdown")
	case <-ctx.Done():
		return nil, kerrors.WithMsg(nil, "Context cancelled")
	case s.ops <- op:
		select {
		case <-ctx.Done():
			return nil, kerrors.WithMsg(nil, "Context cancelled")
		case v := <-res:
			return v.client, v.err
		}
	}
}

// GetBucket returns the bucket of the given name
func (s *service) GetBucket(name string) Bucket {
	return &bucket{
		s:        s,
		name:     name,
		location: s.location,
	}
}

// DelBucket deletes the bucket if it exists
func (s *service) DelBucket(ctx context.Context, name string) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.RemoveBucket(ctx, name); err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return kerrors.WithKind(err, ErrNotFound{}, "Failed to get bucket")
		}
		return kerrors.WithKind(err, ErrClient{}, "Failed to remove bucket")
	}
	return nil
}

type (
	// ObjectInfo is stored object metadata
	ObjectInfo struct {
		Size         int64
		ContentType  string
		ETag         string
		LastModified int64
		UserMeta     map[string]string
	}

	// Dir is a collection of objects in the store at a specified directory
	Dir interface {
		Stat(ctx context.Context, name string) (*ObjectInfo, error)
		Get(ctx context.Context, name string) (io.ReadCloser, *ObjectInfo, error)
		Put(ctx context.Context, name string, contentType string, size int64, userMeta map[string]string, object io.Reader) error
		Del(ctx context.Context, name string) error
		Subdir(name string) Dir
	}

	// Bucket is a collection of items of the object store service
	Bucket interface {
		Dir
		Init(ctx context.Context) error
	}

	bucket struct {
		s        *service
		name     string
		location string
	}

	dir struct {
		prefix string
		base   *bucket
	}
)

// Init creates the bucket if it does not exist
func (b *bucket) Init(ctx context.Context) error {
	client, err := b.s.getClient(ctx)
	if err != nil {
		return err
	}
	exists, err := client.BucketExists(ctx, b.name)
	if err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to get bucket")
	}
	if !exists {
		if err := client.MakeBucket(ctx, b.name, minio.MakeBucketOptions{
			Region: b.location,
		}); err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to create bucket")
		}
	}
	return nil
}

// Stat returns metadata of an object from the bucket
func (b *bucket) Stat(ctx context.Context, name string) (*ObjectInfo, error) {
	client, err := b.s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	info, err := client.StatObject(ctx, b.name, name, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return nil, kerrors.WithKind(err, ErrNotFound{}, "Failed to find object")
		}
		return nil, kerrors.WithKind(err, ErrClient{}, "Failed to stat object")
	}
	return &ObjectInfo{
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified.Unix(),
		UserMeta:     info.UserMetadata,
	}, nil
}

// Get gets an object from the bucket
func (b *bucket) Get(ctx context.Context, name string) (io.ReadCloser, *ObjectInfo, error) {
	client, err := b.s.getClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	obj, err := client.GetObject(ctx, b.name, name, minio.GetObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return nil, nil, kerrors.WithKind(err, ErrNotFound{}, "Failed to find object")
		}
		return nil, nil, kerrors.WithKind(err, ErrClient{}, "Failed to get object")
	}
	info, err := obj.Stat()
	if err != nil {
		return nil, nil, kerrors.WithKind(err, ErrClient{}, "Failed to stat object")
	}
	return obj, &ObjectInfo{
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified.Unix(),
		UserMeta:     info.UserMetadata,
	}, nil
}

// Put puts a new object into the bucket
func (b *bucket) Put(ctx context.Context, name string, contentType string, size int64, userMeta map[string]string, object io.Reader) error {
	client, err := b.s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.PutObject(ctx, b.name, name, object, size, minio.PutObjectOptions{ContentType: contentType, UserMetadata: userMeta}); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to save object to bucket")
	}
	return nil
}

// Del removes an object from the bucket
func (b *bucket) Del(ctx context.Context, name string) error {
	client, err := b.s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.RemoveObject(ctx, b.name, name, minio.RemoveObjectOptions{}); err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return kerrors.WithKind(err, ErrNotFound{}, "Failed to find object")
		}
		return kerrors.WithKind(err, ErrClient{}, "Failed to remove object")
	}
	return nil
}

func (d *dir) Stat(ctx context.Context, name string) (*ObjectInfo, error) {
	return d.base.Stat(ctx, d.prefix+subdirpathSeparator+name)
}

func (d *dir) Get(ctx context.Context, name string) (io.ReadCloser, *ObjectInfo, error) {
	return d.base.Get(ctx, d.prefix+subdirpathSeparator+name)
}

func (d *dir) Put(ctx context.Context, name string, contentType string, size int64, userMeta map[string]string, object io.Reader) error {
	return d.base.Put(ctx, d.prefix+subdirpathSeparator+name, contentType, size, userMeta, object)
}

func (d *dir) Del(ctx context.Context, name string) error {
	return d.base.Del(ctx, d.prefix+subdirpathSeparator+name)
}

func (d *dir) Subdir(prefix string) Dir {
	return &dir{
		prefix: d.prefix + subdirpathSeparator + prefix,
		base:   d.base,
	}
}

func (b *bucket) Subdir(prefix string) Dir {
	return &dir{
		prefix: prefix,
		base:   b,
	}
}
