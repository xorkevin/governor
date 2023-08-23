package objstore

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	subdirpathSeparator = "/"
)

type (
	// Objstore is a service wrapper around a object storage client
	Objstore interface {
		Ping(ctx context.Context) error
		GetBucket(name string) Bucket
		DelBucket(ctx context.Context, name string) error
	}

	objstoreClient struct {
		client *minio.Client
		auth   minioauth
	}

	Service struct {
		lc         *lifecycle.Lifecycle[objstoreClient]
		clientname string
		addr       string
		sslmode    bool
		location   string
		config     governor.SecretReader
		log        *klog.LevelLogger
		hbfailed   int
		hbmaxfail  int
		wg         *ksync.WaitGroup
	}
)

// New creates a new object store service instance
func New() *Service {
	return &Service{
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "9000")
	r.SetDefault("sslmode", false)
	r.SetDefault("location", "us-east-1")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 5)
}

var (
	// ErrConn is returned on an objstore connection error
	ErrConn errConn
	// ErrClient is returned for unknown client errors
	ErrClient errClient
	// ErrNotFound is returned when an object is not found
	ErrNotFound errNotFound
)

type (
	errConn     struct{}
	errClient   struct{}
	errNotFound struct{}
)

func (e errConn) Error() string {
	return "Objstore connection error"
}

func (e errClient) Error() string {
	return "Objstore client error"
}

func (e errNotFound) Error() string {
	return "Object not found"
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r
	s.clientname = r.Config().Hostname + "-" + r.Config().Instance

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.sslmode = r.GetBool("sslmode")
	s.location = r.GetStr("location")
	hbinterval, err := r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config",
		klog.AString("addr", s.addr),
		klog.ABool("sslmode", s.sslmode),
		klog.AString("location", s.location),
		klog.AString("hbinterval", hbinterval.String()),
		klog.AInt("hbmaxfail", s.hbmaxfail),
	)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))

	s.lc = lifecycle.New(
		ctx,
		s.handleGetClient,
		s.closeClient,
		s.handlePing,
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[objstoreClient]) {
	// Check client auth expiry, and reinit client if about to be expired
	client, err := m.Construct(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create objstore client"))
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the store. This allows vault to be temporarily
	// unavailable without disrupting the client connections.
	var username string
	if client != nil {
		err = s.clientPing(ctx, client.client)
		if err == nil {
			s.hbfailed = 0
			return
		}
		username = client.auth.Username
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping objstore"),
			klog.AString("addr", s.addr),
			klog.AString("username", username),
		)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to objstore"),
		klog.AString("addr", s.addr),
		klog.AString("username", username),
	)
	s.hbfailed = 0
	// first invalidate cached secret in order to ensure that construct client
	// will use refreshed auth
	s.config.InvalidateSecret("auth")
	// must stop the client in order to invalidate cached client, and force wait
	// on newly constructed client
	m.Stop(ctx)
}

type (
	minioauth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context, m *lifecycle.State[objstoreClient]) (*objstoreClient, error) {
	var auth minioauth
	{
		client := m.Load(ctx)
		if err := s.config.GetSecret(ctx, "auth", 0, &auth); err != nil {
			return client, kerrors.WithMsg(err, "Invalid secret")
		}
		if auth.Username == "" {
			return client, kerrors.WithKind(nil, governor.ErrInvalidConfig, "Empty auth")
		}
		if client != nil && auth == client.auth {
			return client, nil
		}
	}

	objClient, err := minio.New(s.addr, &minio.Options{
		Creds:  credentials.NewStaticV4(auth.Username, auth.Password, ""),
		Secure: s.sslmode,
	})
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient, "Failed to create objstore client")
	}
	if err := s.clientPing(ctx, objClient); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrConn, "Failed to ping objstore")
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Established connection to objstore",
		klog.AString("addr", s.addr),
		klog.AString("username", auth.Username),
	)

	client := &objstoreClient{
		client: objClient,
		auth:   auth,
	}
	m.Store(client)

	return client, nil
}

func (s *Service) closeClient(ctx context.Context, client *objstoreClient) {
	// client has no close
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, ErrConn, "Objstore service not ready")
	}
	return nil
}

func (s *Service) getClient(ctx context.Context) (*minio.Client, error) {
	if client := s.lc.Load(ctx); client != nil {
		return client.client, nil
	}

	client, err := s.lc.Construct(ctx)
	if err != nil {
		// explicitly return nil in order to prevent usage of any cached client
		return nil, err
	}
	return client.client, nil
}

func (s *Service) clientPing(ctx context.Context, client *minio.Client) error {
	if _, err := client.GetBucketLocation(ctx, "healthcheck-probe-"+s.clientname); err != nil {
		if minio.ToErrorResponse(err).StatusCode != http.StatusNotFound {
			return kerrors.WithKind(err, ErrConn, "Failed to ping objstore")
		}
	}
	return nil
}

func (s *Service) Ping(ctx context.Context) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	return s.clientPing(ctx, client)
}

// GetBucket returns the bucket of the given name
func (s *Service) GetBucket(name string) Bucket {
	return &bucket{
		s:        s,
		name:     name,
		location: s.location,
	}
}

// DelBucket deletes the bucket if it exists
func (s *Service) DelBucket(ctx context.Context, name string) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.RemoveBucket(ctx, name); err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return kerrors.WithKind(err, ErrNotFound, "Failed to get bucket")
		}
		return kerrors.WithKind(err, ErrClient, "Failed to remove bucket")
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
		Ping(ctx context.Context) error
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
		s        *Service
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
		return kerrors.WithKind(err, ErrClient, "Failed to get bucket")
	}
	if !exists {
		if err := client.MakeBucket(ctx, b.name, minio.MakeBucketOptions{
			Region: b.location,
		}); err != nil {
			return kerrors.WithKind(err, ErrClient, "Failed to create bucket")
		}
	}
	return nil
}

func (b *bucket) Ping(ctx context.Context) error {
	return b.s.Ping(ctx)
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
			return nil, kerrors.WithKind(err, ErrNotFound, "Failed to find object")
		}
		return nil, kerrors.WithKind(err, ErrClient, "Failed to stat object")
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
			return nil, nil, kerrors.WithKind(err, ErrNotFound, "Failed to find object")
		}
		return nil, nil, kerrors.WithKind(err, ErrClient, "Failed to get object")
	}
	info, err := obj.Stat()
	if err != nil {
		return nil, nil, kerrors.WithKind(err, ErrClient, "Failed to stat object")
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
		return kerrors.WithKind(err, ErrClient, "Failed to save object to bucket")
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
			return kerrors.WithKind(err, ErrNotFound, "Failed to find object")
		}
		return kerrors.WithKind(err, ErrClient, "Failed to remove object")
	}
	return nil
}

func (d *dir) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
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
