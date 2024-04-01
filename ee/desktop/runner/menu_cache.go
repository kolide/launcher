package runner

import (
	"encoding/json"
	"fmt"
	"sync"
)

// menuItemCache is used to keep track of the last set of items
// seen in the menu bar json, to report any changes detected
type menuItemCache struct {
	mu          sync.RWMutex
	cachedItems map[string]struct{}
}

// menuChangeSet is a simple struct used for reporting changes
type menuChangeSet struct {
	Old string `json:"old_label"`
	New string `json:"new_label"`
}

func newMenuItemCache() *menuItemCache {
	return &menuItemCache{
		cachedItems: make(map[string]struct{}),
	}
}

func (m *menuItemCache) recordMenuUpdates(menuData []byte) ([]menuChangeSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	menuChanges := make([]menuChangeSet, 0)

	parsedMenu := struct {
		Items []struct {
			Label string `json:"label"`
		} `json:"items"`
	}{}

	if err := json.Unmarshal(menuData, &parsedMenu); err != nil {
		return menuChanges, fmt.Errorf("unable to parse menu json for change detection: %w", err)
	}

	newCachedMenuData := make(map[string]struct{})
	// first iterate the new menu items, noting any that are new
	for _, item := range parsedMenu.Items {
		if item.Label == "" { // skip separator sections
			continue
		}

		newCachedMenuData[item.Label] = struct{}{}

		if _, ok := m.cachedItems[item.Label]; ok {
			continue
		}

		// append as a change if the section wasn't previously detected
		menuChanges = append(menuChanges, menuChangeSet{Old: "", New: item.Label})
	}

	// now iterate the previously cached items, noting any that have been deleted
	for item := range m.cachedItems {
		if _, ok := newCachedMenuData[item]; ok {
			continue
		}

		menuChanges = append(menuChanges, menuChangeSet{Old: item, New: ""})
	}

	// reset the cached data for the next run
	m.cachedItems = newCachedMenuData

	return menuChanges, nil
}
