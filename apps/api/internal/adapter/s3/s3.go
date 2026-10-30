// Package s3 keeps photos and videos in an S3 bucket: the bundled Garage (homeloom-garage) or any other
// S3-compatible store. Objects are addressed by path, which every S3 store understands.
package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// callTimeout bounds one request to the store.
const callTimeout = 10 * time.Second

type Store struct {
	client *minio.Client
	bucket string
}

// New takes the store's URL (http://homeloom-garage:3900), its region, the bucket and a key
// pair that may read and write it.
func New(endpoint, region, bucket, keyID, secret string) (*Store, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("S3_ENDPOINT: want an http(s) URL, got %q", endpoint)
	}
	client, err := minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(keyID, secret, ""),
		Secure:       u.Scheme == "https",
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, err
	}
	return &Store{client: client, bucket: bucket}, nil
}

func (s *Store) PutPhoto(ctx context.Context, key string, p domain.Photo) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(p.Data), int64(len(p.Data)), minio.PutObjectOptions{ContentType: p.ContentType})
	return err
}

func (s *Store) GetPhoto(ctx context.Context, key string) (domain.Photo, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return domain.Photo{}, notFound(err)
	}
	defer func() { _ = obj.Close() }()
	info, err := obj.Stat()
	if err != nil {
		return domain.Photo{}, notFound(err)
	}
	// Photos are bounded when they are set; reading one byte more tells a stranger object apart.
	data, err := io.ReadAll(io.LimitReader(obj, domain.MaxPhotoBytes+1))
	if err != nil {
		return domain.Photo{}, err
	}
	if len(data) > domain.MaxPhotoBytes {
		return domain.Photo{}, fmt.Errorf("s3: %s is larger than a photo may be", key)
	}
	return domain.Photo{ContentType: info.ContentType, Data: data}, nil
}

func (s *Store) DeletePhoto(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func notFound(err error) error {
	if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
		return domain.ErrNotFound
	}
	return err
}

// videoTimeout bounds sending a whole video to the store or reading one range of it back; a
// stream also ends with the request it serves.
const videoTimeout = 15 * time.Minute

// videoPartSize is how much of a video is held in memory at a time while it is sent on. Without
// it a stream of unknown length is cut into the largest parts S3 allows.
const videoPartSize = 16 << 20

func (s *Store) PutVideo(ctx context.Context, key, contentType string, r io.Reader) error {
	ctx, cancel := context.WithTimeout(ctx, videoTimeout)
	defer cancel()
	_, err := s.client.PutObject(ctx, s.bucket, key, r, -1, minio.PutObjectOptions{ContentType: contentType, PartSize: videoPartSize})
	return err
}

func (s *Store) StatVideo(ctx context.Context, key string) (domain.Video, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return domain.Video{}, notFound(err)
	}
	return domain.Video{ContentType: info.ContentType, Size: info.Size}, nil
}

func (s *Store) OpenVideo(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, videoTimeout)
	opts := minio.GetObjectOptions{}
	if length > 0 {
		if err := opts.SetRange(offset, offset+length-1); err != nil {
			cancel()
			return nil, err
		}
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, opts)
	if err != nil {
		cancel()
		return nil, notFound(err)
	}
	return stream{Object: obj, cancel: cancel}, nil
}

func (s *Store) DeleteVideo(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// stream is a video being read; closing it ends the read and its timeout.
type stream struct {
	*minio.Object
	cancel context.CancelFunc
}

func (s stream) Close() error {
	defer s.cancel()
	return s.Object.Close()
}
