package objstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/minio/minio-go/v6"
	"xorkevin.dev/governor"
)

type (
	// Objstore is a service wrapper around a object storage client
	Objstore interface {
		GetBucket(name string) Bucket
		DelBucket(name string) error
	}

	// Service is an Objstore and governor.Service
	Service interface {
		governor.Service
		Objstore
	}

	minioauth struct {
		username string
		password string
	}

	getClientRes struct {
		client *minio.Client
		err    error
	}

	getOp struct {
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
		ready      bool
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
		ready:    false,
		hbfailed: 0,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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

	if _, err := s.getClient(); err != nil {
		return err
	}
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
			s.handlePing()
		case op := <-s.ops:
			client, err := s.handleGetClient()
			op.res <- getClientRes{
				client: client,
				err:    err,
			}
			close(op.res)
		}
	}
}

func (s *service) handlePing() {
	if s.client != nil {
		_, err := s.client.ListBuckets()
		if err == nil {
			s.ready = true
			s.hbfailed = 0
			return
		}
		s.hbfailed++
		if s.hbfailed < s.hbmaxfail {
			s.logger.Warn("failed to ping objstore", map[string]string{
				"error":      err.Error(),
				"actiontype": "pingobj",
				"address":    s.addr,
				"username":   s.auth.username,
			})
			return
		}
		s.ready = false
		s.hbfailed = 0
		s.auth = minioauth{}
		s.config.InvalidateSecret("auth")
	}
	if _, err := s.handleGetClient(); err != nil {
		s.logger.Error("failed to create objstore client", map[string]string{
			"error":      err.Error(),
			"actiontype": "createobjclient",
		})
	}
}

func (s *service) handleGetClient() (*minio.Client, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	password, ok := authsecret["password"].(string)
	if !ok || password == "" {
		return nil, governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	auth := minioauth{
		username: "admin",
		password: password,
	}
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	client, err := minio.New(s.addr, auth.username, auth.password, s.sslmode)
	if err != nil {
		return nil, governor.ErrWithKind(err, ErrClient{}, "Failed to create objstore client")
	}
	if _, err := client.ListBuckets(); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, governor.ErrWithKind(err, ErrConn{}, "Failed to ping objstore")
	}

	s.client = client
	s.auth = auth
	s.ready = false
	s.hbfailed = 0
	s.logger.Info(fmt.Sprintf("established connection to %s with key %s", s.addr, s.auth.username), nil)
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
		l.Warn("failed to stop", nil)
	}
}

func (s *service) Health() error {
	if !s.ready {
		return governor.ErrWithKind(nil, ErrConn{}, "Objstore service not ready")
	}
	return nil
}

func (s *service) getClient() (*minio.Client, error) {
	res := make(chan getClientRes)
	op := getOp{
		res: res,
	}
	select {
	case <-s.done:
		return nil, governor.ErrWithKind(nil, ErrConn{}, "Objstore service shutdown")
	case s.ops <- op:
		v := <-res
		return v.client, v.err
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
func (s *service) DelBucket(name string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}
	if err := client.RemoveBucket(name); err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return governor.ErrWithKind(err, ErrNotFound{}, "Failed to get bucket")
		}
		return governor.ErrWithKind(err, ErrClient{}, "Failed to remove bucket")
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
	}

	// Dir is a collection of objects in the store at a specified directory
	Dir interface {
		Stat(name string) (*ObjectInfo, error)
		Get(name string) (io.ReadCloser, *ObjectInfo, error)
		Put(name string, contentType string, size int64, object io.Reader) error
		Del(name string) error
		Subdir(name string) Dir
	}

	// Bucket is a collection of items of the object store service
	Bucket interface {
		Dir
		Init() error
	}

	bucket struct {
		s        *service
		name     string
		location string
	}

	dir struct {
		parent Dir
		name   string
	}
)

// Init creates the bucket if it does not exist
func (b *bucket) Init() error {
	client, err := b.s.getClient()
	if err != nil {
		return err
	}
	exists, err := client.BucketExists(b.name)
	if err != nil {
		return governor.ErrWithKind(err, ErrClient{}, "Failed to get bucket")
	}
	if !exists {
		if err := client.MakeBucket(b.name, b.location); err != nil {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to create bucket")
		}
	}
	return nil
}

// Stat returns metadata of an object from the bucket
func (b *bucket) Stat(name string) (*ObjectInfo, error) {
	client, err := b.s.getClient()
	if err != nil {
		return nil, err
	}
	info, err := client.StatObject(b.name, name, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return nil, governor.ErrWithKind(err, ErrNotFound{}, "Failed to find object")
		}
		return nil, governor.ErrWithKind(err, ErrClient{}, "Failed to stat object")
	}
	return &ObjectInfo{
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified.Unix(),
	}, nil
}

// Get gets an object from the bucket
func (b *bucket) Get(name string) (io.ReadCloser, *ObjectInfo, error) {
	client, err := b.s.getClient()
	if err != nil {
		return nil, nil, err
	}
	obj, err := client.GetObject(b.name, name, minio.GetObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return nil, nil, governor.ErrWithKind(err, ErrNotFound{}, "Failed to find object")
		}
		return nil, nil, governor.ErrWithKind(err, ErrClient{}, "Failed to get object")
	}
	info, err := obj.Stat()
	if err != nil {
		return nil, nil, governor.ErrWithKind(err, ErrClient{}, "Failed to stat object")
	}
	return obj, &ObjectInfo{
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified.Unix(),
	}, nil
}

// Put puts a new object into the bucket
func (b *bucket) Put(name string, contentType string, size int64, object io.Reader) error {
	client, err := b.s.getClient()
	if err != nil {
		return err
	}
	if _, err := client.PutObject(b.name, name, object, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return governor.ErrWithKind(err, ErrClient{}, "Failed to save object to bucket")
	}
	return nil
}

// Del removes an object from the bucket
func (b *bucket) Del(name string) error {
	client, err := b.s.getClient()
	if err != nil {
		return err
	}
	if err := client.RemoveObject(b.name, name); err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return governor.ErrWithKind(err, ErrNotFound{}, "Failed to find object")
		}
		return governor.ErrWithKind(err, ErrClient{}, "Failed to remove object")
	}
	return nil
}

func (d *dir) Stat(name string) (*ObjectInfo, error) {
	return d.parent.Stat(d.name + "/" + name)
}

func (d *dir) Get(name string) (io.ReadCloser, *ObjectInfo, error) {
	return d.parent.Get(d.name + "/" + name)
}

func (d *dir) Put(name string, contentType string, size int64, object io.Reader) error {
	return d.parent.Put(d.name+"/"+name, contentType, size, object)
}

func (d *dir) Del(name string) error {
	return d.parent.Del(d.name + "/" + name)
}

func (d *dir) Subdir(name string) Dir {
	return &dir{
		parent: d,
		name:   name,
	}
}

func (b *bucket) Subdir(name string) Dir {
	return &dir{
		parent: b,
		name:   name,
	}
}
