package email

import (
	"errors"
	"fmt"
	ht "html/template"
	"os"
	"path/filepath"
	"sync"
	tt "text/template"

	"golang.org/x/sync/singleflight"
)

var errTemplateNotFound = errors.New("template not found")

const (
	defaultLocale = "en"
)

type templateID string
type templateLocale string

type templates struct {
	text *tt.Template
	html *ht.Template
}

type templatesCache struct {
	mtx         sync.RWMutex
	data        map[templateID]map[templateLocale]templates
	loaderGroup singleflight.Group
}

type templateManager struct {
	templatesCache templatesCache
	location       string
}

type templateManagerOptions struct {
	Location string
}

func newTemplateManager(options *templateManagerOptions) *templateManager {
	return &templateManager{
		templatesCache: templatesCache{
			data: make(map[templateID]map[templateLocale]templates),
		},
		location: options.Location,
	}
}

func (tm *templateManager) Get(id templateID, locale templateLocale) (templates, error) {
	if locale == "" {
		locale = defaultLocale
	}

	tmpl, ok, err := tm.templatesCache.get(id, locale)
	if err != nil {
		return templates{}, fmt.Errorf("failed to get template %q (%q) from cache: %w", id, locale, err)
	}
	if ok {
		return tmpl, nil
	}

	_, err, _ = tm.templatesCache.loaderGroup.Do(string(id), func() (any, error) {
		loadedTemplates, err := tm.loadTemplates(id)
		if err != nil {
			tm.templatesCache.set(id, make(map[templateLocale]templates)) // mark as not found
			return false, fmt.Errorf("failed to load templates %q: %w", id, err)
		}

		tm.templatesCache.set(id, loadedTemplates)
		return true, nil
	})
	if err != nil {
		return templates{}, fmt.Errorf("failed to load template %q (%q): %w", id, locale, err)
	}

	tmpl, ok, err = tm.templatesCache.get(id, locale)
	if err != nil {
		return templates{}, fmt.Errorf("failed to get template %q (%q) from cache: %w", id, locale, err)
	}
	if !ok {
		return templates{}, fmt.Errorf("template %q not found for locale %q: %w", id, locale, errTemplateNotFound)
	}
	return tmpl, nil
}

func (tm *templatesCache) get(id templateID, locale templateLocale) (templates, bool, error) {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()

	locales, ok := tm.data[id]
	if !ok {
		return templates{}, false, nil
	}

	tmpl, ok := locales[locale]
	if !ok {
		tmpl, ok = locales[defaultLocale]
		if !ok {
			return templates{}, false, fmt.Errorf("template %q not found for locale %q: %w", id, locale, errTemplateNotFound)
		}
		return tmpl, true, nil
	}
	return tmpl, true, nil
}

func (tm *templatesCache) set(id templateID, locales map[templateLocale]templates) {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	tm.data[id] = locales
}

func (tm *templateManager) loadTemplates(id templateID) (map[templateLocale]templates, error) {
	basePath := filepath.Join(tm.location, string(id))

	entries, err := os.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("template %q not found: %w", id, err)
		}
		return nil, fmt.Errorf("failed to read template directory %q: %w", basePath, err)
	}

	result := make(map[templateLocale]templates)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		locale := templateLocale(entry.Name())
		localePath := filepath.Join(basePath, entry.Name())

		var (
			textTpl *tt.Template
			htmlTpl *ht.Template
		)

		// Load text template (index.txt)
		textPath := filepath.Join(localePath, "index.txt")
		if fileExists(textPath) {
			tpl, err := tt.ParseFiles(textPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse text template %q (%s): %w", id, locale, err)
			}
			textTpl = tpl
		}

		// Load HTML template (index.html)
		htmlPath := filepath.Join(localePath, "index.html")
		if fileExists(htmlPath) {
			tpl, err := ht.ParseFiles(htmlPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse html template %q (%s): %w", id, locale, err)
			}
			htmlTpl = tpl
		}

		// Skip locales that contain neither text nor HTML
		if textTpl == nil && htmlTpl == nil {
			return nil, fmt.Errorf("failed to find text and/or html template %q (%s): %w", id, locale, errTemplateNotFound)
		}

		result[locale] = templates{
			text: textTpl,
			html: htmlTpl,
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("template %q contains no valid locale templates: %w", id, errTemplateNotFound)
	}

	return result, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
