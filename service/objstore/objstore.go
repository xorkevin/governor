package objstore

import (
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/minio/minio-go"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
)

const (
	canaryBucket    = "bucket-canary"
	defaultLocation = "us-east-1"
	moduleID        = "objstore"
)

type (
	// Objstore is an object store service
	Objstore struct {
		store *minio.Client
	}

	// Bucket is a collection of items of the object store service
	Bucket struct {
		store    *minio.Client
		name     string
		location string
	}
)

// New creates a new object store service instance
func New(c governor.Config, l *logrus.Logger) (*Objstore, error) {
	v := c.Conf()
	minioconf := v.GetStringMapString("minio")
	client, err := minio.New(minioconf["host"]+":"+minioconf["port"], minioconf["key_id"], minioconf["key_secret"], v.GetBool("minio.sslmode"))
	if err != nil {
		l.Errorf("error creating object store: %s\n", err)
		return nil, err
	}

	client.SetAppInfo(c.Appname, c.Version)

	if err := initBucket(client, canaryBucket, defaultLocation); err != nil {
		l.Error(err.Error())
		return nil, err
	}

	l.Info("initialized object store")
	return &Objstore{
		store: client,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (o *Objstore) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	l.Info("mounted object store")
	return nil
}

const (
	moduleIDHealth = moduleID + ".health"
)

// Health is a health check for the service
func (o *Objstore) Health() *governor.Error {
	if exists, err := o.store.BucketExists(canaryBucket); err != nil || !exists {
		return governor.NewError(moduleIDHealth, err.Error(), 0, http.StatusServiceUnavailable)
	}
	return nil
}

// Setup is run on service setup
func (o *Objstore) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	return nil
}

// GetBucket creates a new bucket if it does not exist in the store and returns the bucket
func (o *Objstore) GetBucket(name, location string) (*Bucket, *governor.Error) {
	if err := initBucket(o.store, name, location); err != nil {
		return nil, err
	}
	return &Bucket{
		store:    o.store,
		name:     name,
		location: location,
	}, nil
}

// DestroyBucket destroys the bucket if it exists
func (o *Objstore) DestroyBucket(name string) *governor.Error {
	exists, err := o.store.BucketExists(name)
	if err != nil {
		return governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}
	if exists {
		if err := o.store.RemoveBucket(name); err != nil {
			return governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
		}
	}
	return nil
}

// GetBucketDefLoc creates a new bucket if it does not exist at the default location in the store and returns the bucket
func (o *Objstore) GetBucketDefLoc(name string) (*Bucket, *governor.Error) {
	return o.GetBucket(name, defaultLocation)
}

func initBucket(client *minio.Client, name, location string) *governor.Error {
	exists, err := client.BucketExists(name)
	if err != nil {
		return governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}
	if !exists {
		if err := client.MakeBucket(name, location); err != nil {
			return governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
		}
	}
	return nil
}

func rmBucket(client *minio.Client, name string) *governor.Error {
	if err := client.RemoveBucket(name); err != nil {
		return governor.NewError(moduleID, "error removing bucket: "+err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDBucket = moduleID + ".Bucket"
)

// Put puts a new object into the bucket
func (b *Bucket) Put(name, contentType string, object io.Reader) *governor.Error {
	if _, err := b.store.PutObject(b.name, name, object, contentType); err != nil {
		return governor.NewError(moduleIDBucket, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

// Get gets an object from the bucket
func (b *Bucket) Get(name string) (*minio.Object, *minio.ObjectInfo, *governor.Error) {
	obj, err := b.store.GetObject(b.name, name)
	if err != nil {
		return nil, nil, governor.NewError(moduleIDBucket, err.Error(), 2, http.StatusNotFound)
	}
	objinfo, err := obj.Stat()
	if err != nil {
		return nil, nil, governor.NewError(moduleIDBucket, err.Error(), 0, http.StatusInternalServerError)
	}
	return obj, &objinfo, nil
}

// Remove removes an object from the bucket
func (b *Bucket) Remove(name string) *governor.Error {
	if err := b.store.RemoveObject(b.name, name); err != nil {
		return governor.NewError(moduleIDBucket, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}