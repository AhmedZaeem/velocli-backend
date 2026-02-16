package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/velocli/velocli/velocli-backend/internal/cryptox"
)

type SelectionMode string

const (
	SelectionModeMulti  SelectionMode = "multi"
	SelectionModeSingle SelectionMode = "single"
)

type Catalog struct {
	Categories    []Category     `json:"categories"`
	Blocks        []Block        `json:"blocks"`
	MainTemplates []MainTemplate `json:"mainTemplates"`
}

type Category struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	SelectionMode SelectionMode `json:"selectionMode"`
}

type Block struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	CategoryID  string            `json:"categoryId"`
	Description string            `json:"description"`
	BasePath    string            `json:"basePath"`
	Conflicts   []string          `json:"conflicts"`
	Deps        map[string]string `json:"deps"`
	MainTarget  string            `json:"mainTarget"`
	MainMode    string            `json:"mainMode"`
	MainContent string            `json:"mainContent"`
	BlobID      string            `json:"blobId"`
	UpdatedAt   string            `json:"updatedAt"`
}

type MainTemplate struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Content string `json:"content"`
}

type Store struct {
	mu sync.RWMutex

	key []byte

	stateEncPath string
	keyPath      string
	blobDir      string

	catalog Catalog

	watchMu    sync.Mutex
	watchNext  int64
	watchers   map[int64]chan struct{}
	catalogVer int64
}

func New(baseDir string, envKey string) (*Store, error) {
	keyPath := filepath.Join(baseDir, "data", ".key")
	key, err := cryptox.LoadKeyFromEnvOrFile(envKey, keyPath)
	if err != nil {
		return nil, err
	}

	s := &Store{
		key:          key,
		keyPath:      keyPath,
		stateEncPath: filepath.Join(baseDir, "data", "state.enc"),
		blobDir:      filepath.Join(baseDir, "data", "blobs"),
		watchers:     map[int64]chan struct{}{},
	}

	if err := s.loadOrInit(filepath.Join(baseDir, "data", "catalog.json")); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) loadOrInit(legacyCatalogPath string) error {
	if b, err := os.ReadFile(s.stateEncPath); err == nil {
		plain, err := cryptox.Decrypt(s.key, b)
		if err != nil {
			return err
		}
		var c Catalog
		if err := json.Unmarshal(plain, &c); err != nil {
			return err
		}
		s.catalog = c
		return nil
	}

	if b, err := os.ReadFile(legacyCatalogPath); err == nil {
		var legacy struct {
			Categories []struct {
				ID            string `json:"id"`
				Name          string `json:"name"`
				SelectionMode string `json:"selectionMode"`
				Features      []struct {
					ID    string `json:"id"`
					Label string `json:"label"`
				} `json:"features"`
			} `json:"categories"`
			MainTemplates []MainTemplate `json:"mainTemplates"`
		}
		if err := json.Unmarshal(b, &legacy); err != nil {
			return err
		}

		cats := make([]Category, 0, len(legacy.Categories))
		for _, cat := range legacy.Categories {
			mode := SelectionMode(cat.SelectionMode)
			if mode != SelectionModeMulti && mode != SelectionModeSingle {
				mode = SelectionModeMulti
			}
			cats = append(cats, Category{
				ID:            cat.ID,
				Name:          cat.Name,
				SelectionMode: mode,
			})
		}

		s.catalog = Catalog{
			Categories:    cats,
			Blocks:        []Block{},
			MainTemplates: legacy.MainTemplates,
		}
		if err := s.saveLocked(); err != nil {
			return err
		}
		_ = os.Remove(legacyCatalogPath)
		return nil
	}

	s.catalog = Catalog{
		Categories: []Category{},
		Blocks:     []Block{},
		MainTemplates: []MainTemplate{
			{ID: "default", Label: "Flutter Default (leave as-is)", Content: ""},
		},
	}
	return s.saveLocked()
}

func (s *Store) GetCatalog() Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.catalog
}

func (s *Store) CatalogVersion() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.catalogVer
}

func (s *Store) SubscribeCatalogChanges() (id int64, ch <-chan struct{}, cancel func()) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	s.watchNext++
	id = s.watchNext
	c := make(chan struct{}, 1)
	s.watchers[id] = c
	return id, c, func() { s.unsubscribe(id) }
}

func (s *Store) unsubscribe(id int64) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if c, ok := s.watchers[id]; ok {
		delete(s.watchers, id)
		close(c)
	}
}

func (s *Store) notifyCatalogChanged() {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	for _, c := range s.watchers {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

func (s *Store) PutCatalog(c Catalog) error {
	for i := range c.Categories {
		mode := c.Categories[i].SelectionMode
		if mode != SelectionModeMulti && mode != SelectionModeSingle {
			return errors.New("invalid selectionMode")
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.catalog.Categories = c.Categories
	s.catalog.MainTemplates = c.MainTemplates
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.notifyCatalogChanged()
	return nil
}

func (s *Store) UpsertBlock(b Block, encryptedBlob []byte) error {
	if b.ID == "" {
		b.ID = randID("blk")
	}
	if b.BlobID == "" {
		b.BlobID = randID("blob")
	}
	if b.UpdatedAt == "" {
		b.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else {
		b.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if err := os.MkdirAll(s.blobDir, 0o755); err != nil {
		return err
	}
	blobPath := filepath.Join(s.blobDir, b.BlobID+".bin")
	if err := writeFileAtomic(blobPath, encryptedBlob, 0o600); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i := range s.catalog.Blocks {
		if s.catalog.Blocks[i].ID == b.ID {
			s.catalog.Blocks[i] = b
			found = true
			break
		}
	}
	if !found {
		s.catalog.Blocks = append(s.catalog.Blocks, b)
	}
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.notifyCatalogChanged()
	return nil
}

func (s *Store) UpsertBlockMeta(b Block) error {
	if b.ID == "" {
		return errors.New("missing id")
	}
	if b.UpdatedAt == "" {
		b.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else {
		b.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.catalog.Blocks {
		if s.catalog.Blocks[i].ID != b.ID {
			continue
		}
		if b.BlobID == "" {
			b.BlobID = s.catalog.Blocks[i].BlobID
		}
		s.catalog.Blocks[i] = b
		if err := s.saveLocked(); err != nil {
			return err
		}
		s.notifyCatalogChanged()
		return nil
	}
	return errors.New("block not found")
}

func (s *Store) DeleteBlock(id string) error {
	if id == "" {
		return errors.New("missing id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	var blobID string
	for i := range s.catalog.Blocks {
		if s.catalog.Blocks[i].ID == id {
			idx = i
			blobID = s.catalog.Blocks[i].BlobID
			break
		}
	}
	if idx == -1 {
		return errors.New("block not found")
	}

	s.catalog.Blocks = append(s.catalog.Blocks[:idx], s.catalog.Blocks[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		return err
	}

	if blobID != "" {
		_ = os.Remove(filepath.Join(s.blobDir, blobID+".bin"))
	}
	s.notifyCatalogChanged()
	return nil
}

func (s *Store) GetEncryptedBlockBlob(blockID string) ([]byte, Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, b := range s.catalog.Blocks {
		if b.ID != blockID {
			continue
		}
		if b.BlobID == "" {
			return nil, Block{}, errors.New("missing blob")
		}
		blobPath := filepath.Join(s.blobDir, b.BlobID+".bin")
		data, err := os.ReadFile(blobPath)
		if err != nil {
			return nil, Block{}, err
		}
		return data, b, nil
	}
	return nil, Block{}, errors.New("block not found")
}

func (s *Store) Encrypt(plaintext []byte) ([]byte, error) {
	return cryptox.Encrypt(s.key, plaintext)
}

func (s *Store) Decrypt(data []byte) ([]byte, error) {
	return cryptox.Decrypt(s.key, data)
}

func (s *Store) saveLocked() error {
	plain, err := json.Marshal(s.catalog)
	if err != nil {
		return err
	}
	enc, err := cryptox.Encrypt(s.key, plain)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(s.stateEncPath, enc, 0o600); err != nil {
		return err
	}
	s.catalogVer++
	return nil
}

func writeFileAtomic(path string, b []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func randID(prefix string) string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return prefix + "_" + hex.EncodeToString(buf[:])
}
