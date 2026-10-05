package store

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"argodeck/internal/argocd"
)

// On-disk snapshot of an instance, so a restart shows the last known state
// immediately instead of waiting for a full list of thousands of apps. The
// live list + watch then replaces it in the background.

const cacheVersion = 1

type cacheFile struct {
	Version  int                     `json:"version"`
	SavedAt  time.Time               `json:"savedAt"`
	Server   string                  `json:"server"`
	Apps     []*argocd.Application   `json:"apps"`
	AppSets  []AppSetSummary         `json:"appSets"`
	Clusters []argocd.Cluster        `json:"clusters"`
	Enrich   map[string]cachedEnrich `json:"enrich,omitempty"` // resource-tree explanations of degraded apps
}

type cachedEnrich struct {
	Stamp    string    `json:"stamp"`
	Problems []Problem `json:"problems"`
}

func (m *Manager) cachePath(id string) string {
	return filepath.Join(m.cfg.Dir(), "cache", id+".json.gz")
}

// loadCache fills the conn from disk and publishes it. Returns false when
// there is no usable cache.
func (c *conn) loadCache() bool {
	f, err := os.Open(c.m.cachePath(c.cfg.ID))
	if err != nil {
		return false
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	var cf cacheFile
	if json.NewDecoder(zr).Decode(&cf) != nil || cf.Version != cacheVersion || cf.Server != c.cfg.Server {
		return false
	}
	c.mu.Lock()
	if len(c.apps) > 0 { // live data already arrived
		c.mu.Unlock()
		return false
	}
	for _, a := range cf.Apps {
		if a != nil {
			c.apps[appKey(c.cfg.ID, a.Metadata.Namespace, a.Metadata.Name)] = a
		}
	}
	ci := clusterInfo{byServer: map[string]argocd.Cluster{}, byName: map[string]argocd.Cluster{}}
	for _, cl := range cf.Clusters {
		ci.byServer[cl.Server] = cl
		if cl.Name != "" {
			ci.byName[cl.Name] = cl
		}
	}
	c.clusters = ci
	for _, s := range cf.AppSets {
		c.appsets[s.Key] = s
	}
	for k, e := range cf.Enrich {
		c.enrich[k] = enrichEntry{stamp: e.Stamp, problems: e.Problems}
	}
	c.status.CachedAt = cf.SavedAt.Format(time.RFC3339)
	c.mu.Unlock()
	c.rebuildAll()
	c.m.emit("appsets", c.m.AppSets())
	c.m.emit("clusters", c.m.Clusters())
	c.m.emitStatus()
	return true
}

func (c *conn) saveCache() error {
	c.mu.RLock()
	cf := cacheFile{Version: cacheVersion, SavedAt: time.Now(), Server: c.cfg.Server,
		Apps: make([]*argocd.Application, 0, len(c.apps))}
	for _, a := range c.apps {
		cf.Apps = append(cf.Apps, a)
	}
	for _, s := range c.appsets {
		cf.AppSets = append(cf.AppSets, s)
	}
	for _, cl := range c.clusters.byServer {
		cf.Clusters = append(cf.Clusters, cl)
	}
	cf.Enrich = make(map[string]cachedEnrich, len(c.enrich))
	for k, e := range c.enrich {
		cf.Enrich[k] = cachedEnrich{Stamp: e.stamp, Problems: e.problems}
	}
	c.mu.RUnlock()
	if len(cf.Apps) == 0 {
		return nil
	}
	path := c.m.cachePath(c.cfg.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	zw, _ := gzip.NewWriterLevel(f, gzip.BestSpeed)
	err = json.NewEncoder(zw).Encode(cf)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	c.dirty.Store(false)
	return os.Rename(tmp, path)
}

// SaveCaches persists every instance (called on shutdown).
func (m *Manager) SaveCaches() {
	m.mu.RLock()
	conns := make([]*conn, 0, len(m.conns))
	for _, c := range m.conns {
		conns = append(conns, c)
	}
	m.mu.RUnlock()
	for _, c := range conns {
		if c.dirty.Load() {
			_ = c.saveCache()
		}
	}
}
