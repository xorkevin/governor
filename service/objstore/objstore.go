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

	service struct {
		location string
		store    *minio.Client
		logger   governor.Logger
	}
)

// New creates a new object store service instance
func New() Service {
	return &service{}
}

func (s *service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("keyid", "admin")
	r.SetDefault("keysecret", "adminsecret")
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
	conf := r.GetStrMap("")
	client, err := minio.New(conf["host"]+":"+conf["port"], conf["keyid"], conf["keysecret"], r.GetBool("sslmode"))
	if err != nil {
		l.Error("failed to create objstore", map[string]string{
			"error": err.Error(),
		})
		return governor.NewError("Failed to create objstore client", http.StatusInternalServerError, err)
	}
	s.store = client
	s.location = conf["location"]

	s.store.SetAppInfo(c.Appname, c.Version)

	if _, err := s.store.ListBuckets(); err != nil {
		l.Error("failed to ping object store", map[string]string{
			"error": err.Error(),
		})
		return governor.NewError("Failed to ping object store", http.StatusInternalServerError, err)
	}

	l.Info(fmt.Sprintf("established objstore connection to %s:%s", conf["host"], conf["port"]), nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	if _, err := s.store.ListBuckets(); err != nil {
		return governor.NewError("Failed to ping object store", http.StatusInternalServerError, err)
	}
	return nil
}

// GetBucket returns the bucket of the given name
func (s *service) GetBucket(name string) Bucket {
	return &bucket{
		service:  s,
		name:     name,
		location: s.location,
	}
}

// DelBucket deletes the bucket if it exists
func (s *service) DelBucket(name string) error {
	if err := s.store.RemoveBucket(name); err != nil {
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
		service  *service
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
	exists, err := b.service.store.BucketExists(b.name)
	if err != nil {
		return governor.NewError("Failed to get bucket", http.StatusInternalServerError, err)
	}
	if !exists {
		if err := b.service.store.MakeBucket(b.name, b.location); err != nil {
			return governor.NewError("Failed to create bucket", http.StatusInternalServerError, err)
		}
	}
	return nil
}

// Stat returns metadata of an object from the bucket
func (b *bucket) Stat(name string) (*ObjectInfo, error) {
	info, err := b.service.store.StatObject(b.name, name, minio.StatObjectOptions{})
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
	info, err := b.Stat(name)
	if err != nil {
		return nil, nil, err
	}

	obj, err := b.service.store.GetObject(b.name, name, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, governor.NewError("Failed to get object", http.StatusInternalServerError, err)
	}
	return obj, info, nil
}

// Put puts a new object into the bucket
func (b *bucket) Put(name string, contentType string, size int64, object io.Reader) error {
	if _, err := b.service.store.PutObject(b.name, name, object, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return governor.NewError("Failed to save object to bucket", http.StatusInternalServerError, err)
	}
	return nil
}

// Del removes an object from the bucket
func (b *bucket) Del(name string) error {
	if err := b.service.store.RemoveObject(b.name, name); err != nil {
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
