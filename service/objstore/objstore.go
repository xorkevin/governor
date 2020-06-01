package objstore

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v6"
	"io"
	"net/http"
	"xorkevin.dev/governor"
)

type (
	// Objstore is a service wrapper around a object storage client
	Objstore interface {
		GetBucket(name string) Bucket
		DelBucket(name string) error
	}

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

	pingOp struct {
		res chan<- error
	}

	service struct {
		client   *minio.Client
		auth     minioauth
		addr     string
		sslmode  bool
		location string
		config   governor.SecretReader
		logger   governor.Logger
		ops      chan getOp
		pings    chan pingOp
		done     <-chan struct{}
	}
)

// New creates a new object store service instance
func New() Service {
	return &service{
		ops:   make(chan getOp),
		pings: make(chan pingOp),
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "9000")
	r.SetDefault("sslmode", false)
	r.SetDefault("location", "us-east-1")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	conf := r.GetStrMap("")
	s.addr = fmt.Sprintf("%s:%s", conf["host"], conf["port"])
	s.sslmode = r.GetBool("sslmode")
	s.location = conf["location"]

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
	for {
		select {
		case <-ctx.Done():
			s.closeClient()
			return
		case op := <-s.pings:
			op.res <- s.handlePing()
			close(op.res)
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

func (s *service) handlePing() error {
	if s.client == nil {
		return governor.NewError("No object store connection", http.StatusInternalServerError, nil)
	}
	if _, err := s.client.ListBuckets(); err != nil {
		s.config.InvalidateSecret("auth")
		return governor.NewError("Failed to ping object store", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) handleGetClient() (*minio.Client, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	auth := minioauth{
		username: authsecret["username"].(string),
		password: authsecret["password"].(string),
	}
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	client, err := minio.New(s.addr, auth.username, auth.password, s.sslmode)
	if err != nil {
		return nil, governor.NewError("Failed to create object store client", http.StatusInternalServerError, err)
	}
	if _, err := client.ListBuckets(); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, governor.NewError("Failed to ping object store", http.StatusInternalServerError, err)
	}

	s.client = client
	s.auth = auth
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
	res := make(chan error)
	op := pingOp{
		res: res,
	}
	select {
	case <-s.done:
		return governor.NewError("Objstore service shutdown", http.StatusInternalServerError, nil)
	case s.pings <- op:
		return <-res
	}
}

func (s *service) getClient() (*minio.Client, error) {
	res := make(chan getClientRes)
	op := getOp{
		res: res,
	}
	select {
	case <-s.done:
		return nil, governor.NewError("Objstore service shutdown", http.StatusInternalServerError, nil)
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
			return governor.NewError("Failed to get bucket", http.StatusNotFound, err)
		}
		return governor.NewError("Failed to remove bucket", http.StatusInternalServerError, err)
	}
	return nil
}

type (
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
		return governor.NewError("Failed to get bucket", http.StatusInternalServerError, err)
	}
	if !exists {
		if err := client.MakeBucket(b.name, b.location); err != nil {
			return governor.NewError("Failed to create bucket", http.StatusInternalServerError, err)
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
			return nil, governor.NewError("Failed to find object", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to stat object", http.StatusInternalServerError, err)
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
			return nil, nil, governor.NewError("Failed to find object", http.StatusNotFound, err)
		}
		return nil, nil, governor.NewError("Failed to get object", http.StatusInternalServerError, err)
	}
	info, err := obj.Stat()
	if err != nil {
		return nil, nil, governor.NewError("Failed to stat object", http.StatusInternalServerError, err)
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
		return governor.NewError("Failed to save object to bucket", http.StatusInternalServerError, err)
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
			return governor.NewError("Failed to find object", http.StatusNotFound, err)
		}
		return governor.NewError("Failed to remove object", http.StatusInternalServerError, err)
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
