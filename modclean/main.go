package main

import (
	"bytes"
	log "github.com/sirupsen/logrus"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type FileInfo struct {
	ModTime  time.Time
	FilePath string
}

func main() {
	cmd := exec.Command("go", "env", "GOMODCACHE")
	cmd.Env = os.Environ()

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		log.Errorf("err:%v", err)
		return
	}

	modRoot := strings.TrimSuffix(filepath.ToSlash(strings.TrimSuffix(out.String(), "\n")), "/") + "/"

	log.Infof("go mod cache is %v", modRoot)

	m := map[string][]FileInfo{}
	err = filepath.Walk(modRoot, func(path string, info fs.FileInfo, err error) error {
		if info == nil {
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		if !strings.Contains(info.Name(), "@") {
			return nil
		}

		modName := strings.Split(info.Name(), "@")[0]
		if modName == "" {
			return nil
		}

		m[modName] = append(m[modName], FileInfo{
			ModTime:  info.ModTime(),
			FilePath: path,
		})
		log.Infof("found mod %s", info.Name())

		return nil
	})
	if err != nil {
		log.Errorf("err:%v", err)
		return
	}

	var removeCount uint32
	for _, values := range m {
		latestAt := time.Time{}
		latestPath := ""

		for _, val := range values {
			if latestAt.Sub(val.ModTime) < 0 {
				if latestPath != "" {
					log.Warnf("will remove %s", latestPath)
					removeCount++
					err = os.RemoveAll(latestPath)
					if err != nil {
						log.Errorf("err:%v", err)
					}
				}

				latestAt = val.ModTime
				latestPath = val.FilePath
			} else {
				log.Warnf("will remove %s", val.FilePath)
				removeCount++
				err = os.RemoveAll(val.FilePath)
				if err != nil {
					log.Errorf("err:%v", err)
				}
			}
		}
	}

	log.Infof("find %v pkg, %v version is removed", len(m), removeCount)
}
