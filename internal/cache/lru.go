package cache

import (
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/chatwoot/dubly/internal/models"
)

type LinkCache struct {
	c *lru.Cache[string, *models.Link]
}

func New(size int) (*LinkCache, error) {
	c, err := lru.New[string, *models.Link](size)
	if err != nil {
		return nil, err
	}
	return &LinkCache{c: c}, nil
}

func key(domain, slug string) string {
	return domain + "/" + slug
}

func (lc *LinkCache) Get(domain, slug string) (*models.Link, bool) {
	return lc.c.Get(key(domain, slug))
}

func (lc *LinkCache) Set(domain, slug string, link *models.Link) {
	lc.c.Add(key(domain, slug), link)
}

func (lc *LinkCache) Invalidate(domain, slug string) {
	lc.c.Remove(key(domain, slug))
}
