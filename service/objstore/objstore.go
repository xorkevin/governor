package objstore

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/minio/minio-go"
	"io"
	"net/http"
)

const (
	canaryBucket    = "bucket-canary"
	defaultLocation = "us-east-1"
)

type (
	// Objstore is an object store service
	Objstore interface {
		governor.Service
		GetBucket(name, location string) (Bucket, error)
		GetBucketDefLoc(name string) (Bucket, error)
		DestroyBucket(name string) error
	}

	minioObjstore struct {
		store *minio.Client
	}

	// Bucket is a collection of items of the object store service
	Bucket interface {
		Put(name, contentType string, size int64, object io.Reader) error
		Stat(name string) (*minio.ObjectInfo, error)
		Get(name string) (*minio.Object, *minio.ObjectInfo, error)
		Remove(name string) error
	}

	minioBucket struct {
		store    *minio.Client
		name     string
		location string
	}
)

// New creates a new object store service instance
func New(c governor.Config, l governor.Logger) (Objstore, error) {
	v := c.Conf()
	minioconf := v.GetStringMapString("minio")
	client, err := minio.New(minioconf["host"]+":"+minioconf["port"], minioconf["key_id"], minioconf["key_secret"], v.GetBool("minio.sslmode"))
	if err != nil {
		l.Error("objstore: fail create objstore", map[string]string{
			"err": err.Error(),
		})
		return nil, err
	}

	client.SetAppInfo(c.Appname, c.Version)

	if err := initBucket(client, canaryBucket, defaultLocation); err != nil {
		l.Error("objstore: fail create objstore", map[string]string{
			"err": err.Error(),
		})
		return nil, err
	}

	l.Info(fmt.Sprintf("objstore: establish objstore connection to %s:%s", minioconf["host"], minioconf["port"]), nil)
	l.Info("initialize objstore service", nil)
	return &minioObjstore{
		store: client,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (o *minioObjstore) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mount objstore service", nil)
	return nil
}

// Health is a health check for the service
func (o *minioObjstore) Health() error {
	if exists, err := o.store.BucketExists(canaryBucket); err != nil || !exists {
		return governor.NewError("Failed to find canary bucket", http.StatusServiceUnavailable, err)
	}
	return nil
}

// Setup is run on service setup
func (o *minioObjstore) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	return nil
}

// GetBucket creates a new bucket if it does not exist in the store and returns the bucket
func (o *minioObjstore) GetBucket(name, location string) (Bucket, error) {
	if err := initBucket(o.store, name, location); err != nil {
		return nil, err
	}
	return &minioBucket{
		store:    o.store,
		name:     name,
		location: location,
	}, nil
}

// DestroyBucket destroys the bucket if it exists
func (o *minioObjstore) DestroyBucket(name string) error {
	exists, err := o.store.BucketExists(name)
	if err != nil {
		return governor.NewError("Failed to find bucket", http.StatusNotFound, err)
	}
	if exists {
		if err := o.store.RemoveBucket(name); err != nil {
			return governor.NewError("Failed to remove bucket", http.StatusInternalServerError, err)
		}
	}
	return nil
}

// GetBucketDefLoc creates a new bucket if it does not exist at the default location in the store and returns the bucket
func (o *minioObjstore) GetBucketDefLoc(name string) (Bucket, error) {
	return o.GetBucket(name, defaultLocation)
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
func (b *minioBucket) Put(name, contentType string, size int64, object io.Reader) error {
	if _, err := b.store.PutObject(b.name, name, object, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return governor.NewError("Failed to add object in bucket", http.StatusInternalServerError, err)
	}
	return nil
}

// Stat returns metadata of an object from the bucket
func (b *minioBucket) Stat(name string) (*minio.ObjectInfo, error) {
	objInfo, err := b.store.StatObject(b.name, name, minio.StatObjectOptions{})
	if err != nil {
		return nil, governor.NewError("Failed to find object", http.StatusNotFound, err)
	}
	return &objInfo, nil
}

// Get gets an object from the bucket
func (b *minioBucket) Get(name string) (*minio.Object, *minio.ObjectInfo, error) {
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
func (b *minioBucket) Remove(name string) error {
	if err := b.store.RemoveObject(b.name, name); err != nil {
		return governor.NewError("Failed to remove object", http.StatusInternalServerError, err)
	}
	return nil
}
