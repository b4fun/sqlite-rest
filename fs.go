package main

import (
	"os"
	"sync"
)

func readFileWithStatCache(file string) func() ([]byte, error) {
	mu := new(sync.RWMutex)
	var (
		lastReadContent []byte
		lastStat os.FileInfo
	)

	fast := func() (bool, []byte, error) {
		stat, err := os.Stat(file)
		if err != nil {
			return false, nil, err
		}

		mu.RLock()
		defer mu.RUnlock()

		if lastStat == nil {
			// no cache
			return false, nil, nil
		}

		if lastStat.ModTime() == stat.ModTime() {
			return true, lastReadContent, nil
		}

		// mod time changed
		return false, nil, nil
	}

	slow := func() ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()

		stat, err := os.Stat(file)
		if err != nil {
			return nil, err
		}

		if lastStat != nil && lastStat.ModTime() == stat.ModTime() {
			return lastReadContent, nil
		}

		lastStat = stat
		lastReadContent = nil

		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		lastReadContent = content

		return content, nil
	}

	return func() ([]byte, error) {
		readFromCache, content, err := fast()
		if err != nil {
			return nil, err
		}
		if readFromCache {
			return content, nil
		}

		return slow()
	}
}