package memory

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Videos is an in-memory port.VideoStore.
type Videos struct {
	mu     sync.Mutex
	videos map[string]video
}

type video struct {
	contentType string
	data        []byte
}

func NewVideos() *Videos { return &Videos{videos: map[string]video{}} }

func (s *Videos) PutVideo(_ context.Context, key, contentType string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.videos[key] = video{contentType: contentType, data: data}
	return nil
}

func (s *Videos) StatVideo(_ context.Context, key string) (domain.Video, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.videos[key]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return domain.Video{ContentType: v.contentType, Size: int64(len(v.data))}, nil
}

func (s *Videos) OpenVideo(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.videos[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	end := min(offset+length, int64(len(v.data)))
	return io.NopCloser(bytes.NewReader(v.data[offset:end])), nil
}

func (s *Videos) DeleteVideo(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.videos, key)
	return nil
}

// Len is how many videos it holds.
func (s *Videos) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.videos)
}
