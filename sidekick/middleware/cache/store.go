package cache


import (
	"os"
	"go.uber.org/zap"
	"strings"
	"errors"
	"time"
	"strconv"
)

type Store struct {
	loc string
	ttl int
	logger *zap.Logger
	memCache map[string]*MemCacheItem
}

type MemCacheItem struct {
	content map[int]*string
	timestamp int64
}

const (
	CACHE_DIR = "sidekick-cache"
)


func NewStore(loc string, ttl int, logger *zap.Logger) *Store {
	os.MkdirAll(loc+"/"+CACHE_DIR, os.ModePerm)
	memCache := make(map[string]*MemCacheItem)

	// // Load cache from disk
	files, err := os.ReadDir(loc+"/"+CACHE_DIR)
	if err == nil {
		for _, file := range files {
			if file.IsDir() {
				pageFiles, err := os.ReadDir(loc+"/"+CACHE_DIR+"/"+file.Name())
				if err != nil {
					continue
				}

				memCache[file.Name()] = &MemCacheItem{
					content: make(map[int]*string),
					timestamp: time.Now().Unix(),
				}

				for idx, pageFile := range pageFiles {
					if !pageFile.IsDir() {
						value, err := os.ReadFile(loc+"/"+CACHE_DIR+"/"+file.Name()+"/"+pageFile.Name())

						if err != nil {
							continue
						}
						newValue := string(value)
						memCache[file.Name()].content[idx] = &newValue
					}
				}
			}
		}
	}

	return &Store{
		loc: loc,
		ttl: ttl,
		logger: logger,
		memCache: memCache,
	}
}


func (d *Store) Get(key string) ([]byte, error) {
	key = strings.ReplaceAll(key, "/", "+")
	d.logger.Debug("Getting key from cache", zap.String("key", key))

	if d.memCache[key] != nil {
		d.logger.Debug("Pulled key from memory", zap.String("key", key))

		if time.Now().Unix() - d.memCache[key].timestamp > int64(d.ttl) {
			d.logger.Debug("Cache expired", zap.String("key", key))
			go d.Purge(key)
			return nil, errors.New("Cache expired")
		}

		content := ""

		for idx := 0; idx < len(d.memCache[key].content); idx++ {
			if d.memCache[key].content[idx] == nil {
				d.logger.Debug("Content missing", zap.Int("index", idx))
				continue
			}

			content += *d.memCache[key].content[idx]
		}

		d.logger.Debug("Cache hit", zap.String("key", key))
		return []byte(content), nil

		//return content, nil
	}

	// load files in directory
	files, err := os.ReadDir(d.loc+"/"+CACHE_DIR+"/"+key)
	if err != nil {
		return nil, errors.New("Key not found in cache")
	}

	content := ""

	for _, file := range files {
		if !file.IsDir() {
			value, err := os.ReadFile(d.loc+"/"+CACHE_DIR+"/"+key+"/"+file.Name())
			if err != nil {
				return nil, errors.New("Key not found in cache")
			}

			content += string(value)
		}
	}

	return []byte(content), nil
}

func (d *Store) Set(key string, idx int, value []byte) error {
	key = strings.ReplaceAll(key, "/", "+")

	if d.memCache[key] == nil {
		d.memCache[key] = &MemCacheItem{
			content: make(map[int]*string),
			timestamp: time.Now().Unix(),
		}
	}
	
	d.logger.Debug("-----------------------------------")
	d.logger.Debug("Setting key in cache", zap.String("key", key))
	d.logger.Debug("Index", zap.Int("index", idx))
	newValue := string(value)
	d.memCache[key].content[idx] = &newValue

	// create page directory 
	os.MkdirAll(d.loc+"/"+CACHE_DIR+"/"+key, os.ModePerm)
	err := os.WriteFile(d.loc+"/"+CACHE_DIR+"/"+key+"/"+strconv.Itoa(idx), value, os.ModePerm)
	
	if err != nil {
		d.logger.Error("Error writing to cache", zap.Error(err))
	}

	return nil
}

func (d *Store) Purge(key string) {
	key = strings.ReplaceAll(key, "/", "+")
	removeLoc := d.loc+"/"+CACHE_DIR+"/."
	d.logger.Debug("Removing key from cache", zap.String("key", key))

	delete(d.memCache, "br::"+key)
	delete(d.memCache, "gzip::"+key)
	
	if _, err := os.Stat(removeLoc+"br::"+key); err == nil {
		d.logger.Info("Removing brotli cache")
		err = os.Remove(removeLoc+"br::"+key)
		d.logger.Info("Brotli remove error status", zap.Error(err))
	}

	if _, err := os.Stat(removeLoc+"gzip::"+key); err == nil {
		d.logger.Info("Removing gzip cache")
		err = os.Remove(removeLoc+"gzip::"+key)
		d.logger.Info("Gzip remove error status", zap.Error(err))
	}
}

func (d *Store) Flush() error {
	d.memCache = make(map[string]*MemCacheItem)
	err := os.RemoveAll(d.loc + "/" + CACHE_DIR)

	if err == nil {
		os.MkdirAll(d.loc+"/"+CACHE_DIR, os.ModePerm)
	} else {
		d.logger.Error("Error flushing cache", zap.Error(err))
	}

	return err
}

func (d *Store) List() map[string][]string {
	list := make(map[string][]string)
	list["mem"] = make([]string, len(d.memCache))
	memIdx := 0

	for key, _ := range d.memCache {
		list["mem"][memIdx] = key
		memIdx++
	}

	files, err := os.ReadDir(d.loc+"/"+CACHE_DIR)
	list["disk"] = make([]string, 0)

	if err == nil {
		for _, file := range files {
			if !file.IsDir() {
				list["disk"] = append(list["disk"], file.Name())
			}
		}
	}

	return list
}