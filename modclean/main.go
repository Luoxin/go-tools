package main

import (
	"bytes"
	"fmt"
	"github.com/pterm/pterm"
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
	Size     int64
}

func main() {
	cmd := exec.Command("go", "env", "GOMODCACHE")
	cmd.Env = os.Environ()

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		pterm.Error.Sprintf("err:%v", err)
		return
	}

	modRoot := strings.TrimSuffix(filepath.ToSlash(strings.TrimSuffix(out.String(), "\n")), "/") + "/"

	pterm.Success.Printfln("go mod cache is %v", modRoot)

	bar, _ := pterm.DefaultSpinner.WithText("Scanning go.mod...").Start()
	m := map[string][]FileInfo{}
	err = filepath.Walk(modRoot, func(path string, info fs.FileInfo, err error) error {
		if info == nil {
			return nil
		}

		if !info.IsDir() {
			bar.UpdateText(pterm.FgLightMagenta.Sprintf("%v is not dir,skip", info.Name()))
			return nil
		}

		if !strings.Contains(info.Name(), "@") {
			bar.UpdateText(pterm.FgLightMagenta.Sprintf("%v is not go.mod path,skip", info.Name()))
			return nil
		}

		modName := strings.Split(info.Name(), "@")[0]
		if modName == "" {
			bar.UpdateText(pterm.FgLightMagenta.Sprintf("%v is not go.mod path,skip", info.Name()))
			return nil
		}

		m[modName] = append(m[modName], FileInfo{
			ModTime:  info.ModTime(),
			FilePath: path,
			Size:     info.Size(),
		})

		bar.UpdateText(pterm.FgLightCyan.Sprintf("found go.mod path %s", info.Name()))

		return nil
	})
	if err != nil {
		bar.FailPrinter.Printfln("err:%v", err)
		return
	}
	bar.Success("scanned go.mod")

	var allRemoveCount uint32
	var allRemoveSize int64
	bar, _ = pterm.DefaultSpinner.WithText("Scanning go.mod...").Start()
	for key, values := range m {
		lastVal := FileInfo{}

		var removeCount uint32
		var removeSize int64

		remove := func(val FileInfo) {
			bar.UpdateText(pterm.FgYellow.Sprintf("will remove %s", val.FilePath))

			size := DirSizeB(val.FilePath)
			if size == 0 {
				size = val.Size
			}

			err = os.RemoveAll(val.FilePath)
			if err != nil {
				pterm.Error.Printfln("err:%v", err)
				return
			}

			allRemoveCount++
			allRemoveSize += size

			removeCount++
			removeSize += size
		}

		for _, val := range values {
			if lastVal.ModTime.Sub(val.ModTime) < 0 {
				if lastVal.FilePath != "" {
					remove(lastVal)
				}

				lastVal = val
			} else {
				remove(val)
			}
		}

		bar.Success(pterm.FgLightGreen.Sprintf("%v clean %v version size %v", key, removeCount, formatFileSize(removeSize)))
	}

	bar.Success(pterm.FgLightGreen.Sprintf("find %v pkg, %v version is removed siz %v", len(m), allRemoveCount, formatFileSize(allRemoveSize)))
}

func formatFileSize(fileSize int64) (size string) {
	if fileSize < 1024 {
		//return strconv.FormatInt(fileSize, 10) + "B"
		return fmt.Sprintf("%.2fB", float64(fileSize)/float64(1))
	} else if fileSize < (1024 * 1024) {
		return fmt.Sprintf("%.2fKB", float64(fileSize)/float64(1024))
	} else if fileSize < (1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fMB", float64(fileSize)/float64(1024*1024))
	} else if fileSize < (1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fGB", float64(fileSize)/float64(1024*1024*1024))
	} else if fileSize < (1024 * 1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fTB", float64(fileSize)/float64(1024*1024*1024*1024))
	} else { //if fileSize < (1024 * 1024 * 1024 * 1024 * 1024 * 1024)
		return fmt.Sprintf("%.2fEB", float64(fileSize)/float64(1024*1024*1024*1024*1024))
	}
}

func DirSizeB(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size
}

func getFileSize(path string) int64 {
	if !exists(path) {
		return 0
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fileInfo.Size()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}
