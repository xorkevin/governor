package objstore

import (
	"context"
	"fmt"
	"github.com/labstack/echo"
	"github.com/minio/minio-go"
	"io"
	"net/http"
	"xorkevin.dev/governor"
)

const (
	canaryBucket    = "bucket-canary"
	defaultLocation = "us-east-1"
)

type (
	// Objstore is a service wrapper around a object storage client
	Objstore interface {
		governor.Service
		GetBucket(name, location string) (Bucket, error)
		GetBucketDefLoc(name string) (Bucket, error)
		DestroyBucket(name string) error
	}

	service struct {
		store  *minio.Client
		logger governor.Logger
	}

	// Bucket is a collection of items of the object store service
	Bucket interface {
		Put(name, contentType string, size int64, object io.Reader) error
		Stat(name string) (*minio.ObjectInfo, error)
		Get(name string) (*minio.Object, *minio.ObjectInfo, error)
		Remove(name string) error
	}

	bucket struct {
		store    *minio.Client
		name     string
		location string
	}
)

// New creates a new object store service instance
func New() Objstore {
	return &service{}
}

func (s *service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("keyid", "admin")
	r.SetDefault("keysecret", "adminsecret")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "9000")
	r.SetDefault("sslmode", false)
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	conf := r.GetStrMap("")
	client, err := minio.New(conf["host"]+":"+conf["port"], conf["keyid"], conf["keysecret"], r.GetBool("sslmode"))
	if err != nil {
		s.logger.Error("objstore: failed to create objstore", map[string]string{
			"error": err.Error(),
		})
		return governor.NewError("Failed to create objstore client", http.StatusInternalServerError, err)
	}
	s.store = client

	client.SetAppInfo(c.Appname, c.Version)

	if err := initBucket(client, canaryBucket, defaultLocation); err != nil {
		s.logger.Error("objstore: failed to create canary bucket", map[string]string{
			"err": err.Error(),
		})
		return governor.NewError("Failed to ping object store", http.StatusInternalServerError, err)
	}

	s.logger.Info(fmt.Sprintf("objstore: establish objstore connection to %s:%s", conf["host"], conf["port"]), nil)
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
	if exists, err := s.store.BucketExists(canaryBucket); err != nil || !exists {
		return governor.NewError("Failed to find canary bucket", http.StatusServiceUnavailable, err)
	}
	return nil
}

// GetBucket creates a new bucket if it does not exist in the store and returns the bucket
func (s *service) GetBucket(name, location string) (Bucket, error) {
	if err := initBucket(s.store, name, location); err != nil {
		return nil, err
	}
	return &bucket{
		store:    s.store,
		name:     name,
		location: location,
	}, nil
}

// DestroyBucket destroys the bucket if it exists
func (s *service) DestroyBucket(name string) error {
	exists, err := s.store.BucketExists(name)
	if err != nil {
		return governor.NewError("Failed to find bucket", http.StatusNotFound, err)
	}
	if exists {
		if err := s.store.RemoveBucket(name); err != nil {
			return governor.NewError("Failed to remove bucket", http.StatusInternalServerError, err)
		}
	}
	return nil
}

// GetBucketDefLoc creates a new bucket if it does not exist at the default location in the store and returns the bucket
func (s *service) GetBucketDefLoc(name string) (Bucket, error) {
	return s.GetBucket(name, defaultLocation)
}

func initBucket(client *minio.Client, name, location string) error {
	exists, err := client.BucketExists(name)
	if err != nil {
		return governor.NewError("Failed to query if bucket exists", http.StatusNotFound, err)
	}
	if !exists {
		if err := client.MakeBucket(name, location); err != nil {
			return governor.NewError("Failed to create bucket", http.StatusInternalServerError, err)
		}
	}
	return nil
}

func rmBucket(client *minio.Client, name string) error {
	if err := client.RemoveBucket(name); err != nil {
		return governor.NewError("Error removing bucket", http.StatusInternalServerError, err)
	}
	return nil
}

// Put puts a new object into the bucket
func (b *bucket) Put(name, contentType string, size int64, object io.Reader) error {
	if _, err := b.store.PutObject(b.name, name, object, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return governor.NewError("Failed to add object in bucket", http.StatusInternalServerError, err)
	}
	return nil
}

// Stat returns metadata of an object from the bucket
func (b *bucket) Stat(name string) (*minio.ObjectInfo, error) {
	objInfo, err := b.store.StatObject(b.name, name, minio.StatObjectOptions{})
	if err != nil {
		return nil, governor.NewError("Failed to find object", http.StatusNotFound, err)
	}
	return &objInfo, nil
}

// Get gets an object from the bucket
func (b *bucket) Get(name string) (*minio.Object, *minio.ObjectInfo, error) {
	objInfo, err := b.store.StatObject(b.name, name, minio.StatObjectOptions{})
	if err != nil {
		return nil, nil, governor.NewError("Failed to find object", http.StatusNotFound, err)
	}
	obj, err := b.store.GetObject(b.name, name, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, governor.NewError("Failed to retrieve object", http.StatusInternalServerError, err)
	}
	return obj, &objInfo, nil
}

// Remove removes an object from the bucket
func (b *bucket) Remove(name string) error {
	if err := b.store.RemoveObject(b.name, name); err != nil {
		return governor.NewError("Failed to remove object", http.StatusInternalServerError, err)
	}
	return nil
}
