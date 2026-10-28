package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var getenv = os.Getenv

type store struct {
	mu       sync.RWMutex
	dir      string
	docs     map[string]document
	profiles []profile
}

func atomicJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".saving-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(name, path)
}
func openStore(dir string) (*store, error) {
	if e := os.MkdirAll(filepath.Join(dir, "documents"), 0700); e != nil {
		return nil, e
	}
	s := &store{dir: dir, docs: map[string]document{}, profiles: defaults()}
	if b, e := os.ReadFile(filepath.Join(dir, "profiles.json")); e == nil {
		if e = json.Unmarshal(b, &s.profiles); e != nil {
			return nil, errors.New("Stored profiles could not be read")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	files, e := filepath.Glob(filepath.Join(dir, "documents", "*.json"))
	if e != nil {
		return nil, e
	}
	for _, file := range files {
		b, e := os.ReadFile(file)
		if e != nil {
			return nil, e
		}
		var d document
		if e = json.Unmarshal(b, &d); e != nil || d.ID == "" {
			return nil, errors.New("A stored document could not be read")
		}
		if d.Classes == nil {
			d.Classes = map[string]classification{}
		}
		if d.Research == nil {
			d.Research = map[string]research{}
		}
		changed := false
		for k, r := range d.Research {
			if r.Status == "running" {
				r.Status = "failed"
				r.Error = "Research interrupted by restart. Retry it."
				d.Research[k] = r
				changed = true
			}
		}
		if d.Status != "ready" && d.Status != "failed" {
			d.Status = "failed"
			d.Error = "Import interrupted by restart. Retry this article."
			changed = true
		}
		for k, c := range d.Classes {
			if c.Status == "queued" || c.Status == "running" {
				c.Status = "failed"
				c.Error = "Classification interrupted by restart. Retry it."
				d.Classes[k] = c
				changed = true
			}
		}
		s.docs[d.ID] = d
		if changed {
			if e = atomicJSON(file, d); e != nil {
				return nil, e
			}
		}
	}
	if e = atomicJSON(filepath.Join(dir, "profiles.json"), s.profiles); e != nil {
		return nil, e
	}
	return s, nil
}
func copyDocument(d document) document {
	b, _ := json.Marshal(d)
	var out document
	json.Unmarshal(b, &out)
	return out
}
func (s *store) get(id string) (document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[id]
	return copyDocument(d), ok
}
func (s *store) list() ([]document, []profile) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	docs := make([]document, 0, len(s.docs))
	for _, d := range s.docs {
		v := copyDocument(d)
		v.Source.Text = ""
		v.Markdown = ""
		for key, c := range v.Classes {
			for _, p := range s.profiles {
				if p.ID == key {
					c.Stale = c.ProfileVersion != p.Version || c.SummaryHash != d.Summary.hash()
				}
			}
			v.Classes[key] = c
		}
		docs = append(docs, v)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Created > docs[j].Created })
	return docs, append([]profile{}, s.profiles...)
}
func (s *store) update(id string, change func(*document)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok {
		return errors.New("Document not found")
	}
	d = copyDocument(d)
	change(&d)
	if e := atomicJSON(filepath.Join(s.dir, "documents", id+".json"), d); e != nil {
		return errors.New("Document could not be saved")
	}
	s.docs[id] = d
	return nil
}
func (s *store) add(url string) (document, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.docs {
		if d.URL == url {
			return copyDocument(d), false, nil
		}
	}
	if len(s.docs) >= 120 {
		return document{}, false, errors.New("Library is limited to 120 articles in this workshop")
	}
	d := document{ID: id(), URL: url, Status: "queued", Created: stamp(), Events: []event{{Stage: "queued", Message: "Waiting for the retrieval agent", At: stamp()}}, Classes: map[string]classification{}, Research: map[string]research{}}
	if e := atomicJSON(filepath.Join(s.dir, "documents", d.ID+".json"), d); e != nil {
		return d, false, errors.New("Document could not be saved")
	}
	s.docs[d.ID] = d
	return copyDocument(d), true, nil
}
func (s *store) profile(id string) (profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.profiles {
		if p.ID == id {
			return p, true
		}
	}
	return profile{}, false
}
func (s *store) saveProfile(p profile) (profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(p.Name) < 2 || len(p.Name) > 80 || len(p.Criteria) < 20 || len(p.Criteria) > 2000 {
		return p, errors.New("Use a name of 2–80 characters and criteria of 20–2000 characters")
	}
	next := append([]profile{}, s.profiles...)
	found := false
	for i, old := range next {
		if p.ID == old.ID {
			p.Version = old.Version + 1
			next[i] = p
			found = true
		}
	}
	if !found {
		if p.ID != "" {
			return p, errors.New("Profile not found")
		}
		if len(next) >= 8 {
			return p, errors.New("This workshop supports up to eight client profiles")
		}
		p.ID = id()
		p.Version = 1
		next = append(next, p)
	}
	if e := atomicJSON(filepath.Join(s.dir, "profiles.json"), next); e != nil {
		return p, errors.New("Profile could not be saved")
	}
	s.profiles = next
	return p, nil
}
