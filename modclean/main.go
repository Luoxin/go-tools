package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/darabuchi/enputi/utils"
	"github.com/darabuchi/log"
	"github.com/elliotchance/pie/pie"
	"github.com/karrick/godirwalk"
	"github.com/pterm/pterm"
	"github.com/rogpeppe/go-internal/modfile"
)

type ModInfo struct {
	ModTime      time.Time
	FilePathList []string
	Version      string
	Name         string
}

type GoEnv struct {
	AR           string `json:"AR"`
	CC           string `json:"CC"`
	CGOCFLAGS    string `json:"CGO_CFLAGS"`
	CGOCPPFLAGS  string `json:"CGO_CPPFLAGS"`
	CGOCXXFLAGS  string `json:"CGO_CXXFLAGS"`
	CGOENABLED   string `json:"CGO_ENABLED"`
	CGOFFLAGS    string `json:"CGO_FFLAGS"`
	CGOLDFLAGS   string `json:"CGO_LDFLAGS"`
	CXX          string `json:"CXX"`
	GCCGO        string `json:"GCCGO"`
	GO111MODULE  string `json:"GO111MODULE"`
	GOARCH       string `json:"GOARCH"`
	GOBIN        string `json:"GOBIN"`
	GOCACHE      string `json:"GOCACHE"`
	GOENV        string `json:"GOENV"`
	GOEXE        string `json:"GOEXE"`
	GOEXPERIMENT string `json:"GOEXPERIMENT"`
	GOFLAGS      string `json:"GOFLAGS"`
	GOGCCFLAGS   string `json:"GOGCCFLAGS"`
	GOHOSTARCH   string `json:"GOHOSTARCH"`
	GOHOSTOS     string `json:"GOHOSTOS"`
	GOINSECURE   string `json:"GOINSECURE"`
	GOMOD        string `json:"GOMOD"`
	GOMODCACHE   string `json:"GOMODCACHE"`
	GONOPROXY    string `json:"GONOPROXY"`
	GONOSUMDB    string `json:"GONOSUMDB"`
	GOOS         string `json:"GOOS"`
	GOPATH       string `json:"GOPATH"`
	GOPRIVATE    string `json:"GOPRIVATE"`
	GOPROXY      string `json:"GOPROXY"`
	GOROOT       string `json:"GOROOT"`
	GOSUMDB      string `json:"GOSUMDB"`
	GOTMPDIR     string `json:"GOTMPDIR"`
	GOTOOLDIR    string `json:"GOTOOLDIR"`
	GOVCS        string `json:"GOVCS"`
	GOVERSION    string `json:"GOVERSION"`
	PKGCONFIG    string `json:"PKG_CONFIG"`
}

var cmdArgs struct {
	DeepClean bool `arg:"-d,--deep-clean" help:"deep clean"`
	OnlyShow  bool `arg:"-s,--only-show" help:"only show"`
	Debug     bool `arg:"-v,--verbose" help:"verbose"`
}

type DeviceItem struct {
	Path  string
	IsDir bool
}

type WorkPool struct {
	data chan interface{}
	w    sync.WaitGroup
	// lock  sync.Mutex
	logic func(i interface{})
}

func NewWorkPool(max int, logic func(i interface{})) *WorkPool {
	pool := &WorkPool{
		data:  make(chan interface{}, 1000),
		logic: logic,
	}

	for i := 0; i < max; i++ {
		exitSign := utils.GetExitSign()
		go func(exitSign chan os.Signal) {
			for {
				select {
				case x := <-pool.data:
					pool.logic(x)
					pool.w.Done()
				case <-exitSign:
					return
				}
			}
		}(exitSign)
	}

	return pool
}

func (p *WorkPool) Submit(i interface{}) {
	p.w.Add(1)
	go func() {
		p.data <- i
	}()
}

func (p *WorkPool) Wait() {
	p.w.Wait()
}

func main() {
	log.SetOutput(ioutil.Discard)
	arg.MustParse(&cmdArgs)
	if cmdArgs.Debug {
		pterm.EnableDebugMessages()
	}

	ptermLogo, _ := pterm.DefaultBigText.WithLetters(
		pterm.NewLettersFromStringWithStyle("mod", pterm.NewStyle(pterm.FgLightCyan)),
		pterm.NewLettersFromStringWithStyle("cache", pterm.NewStyle(pterm.FgLightYellow)),
		pterm.NewLettersFromStringWithStyle("clean", pterm.NewStyle(pterm.FgLightMagenta))).
		Srender()

	pterm.DefaultCenter.Print(ptermLogo)

	pterm.DefaultCenter.Print(pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgLightBlue)).WithMargin(10).Sprint("go.mod cache clean for old version cache"))

	cmd := exec.Command("go", "env", "-json")
	cmd.Env = os.Environ()

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		pterm.Error.Sprintf("err:%v", err)
		return
	}

	var goEnv GoEnv
	err = json.Unmarshal(out.Bytes(), &goEnv)
	if err != nil {
		pterm.Error.Sprintf("get go mod cache path err:%v", err)
		return
	}

	modRoot := filepath.ToSlash(goEnv.GOMODCACHE)

	pterm.Success.Printfln("go mod cache is %v", modRoot)

	cacheMap := map[string][]*ModInfo{}

	bar, _ := pterm.DefaultSpinner.WithText("scanning mod cache...").Start()
	err = godirwalk.Walk(modRoot, &godirwalk.Options{
		ErrorCallback: func(s string, err error) godirwalk.ErrorAction {
			return godirwalk.SkipNode
		},
		Unsorted: true,
		Callback: func(osPathname string, info *godirwalk.Dirent) error {
			if info == nil {
				return nil
			}

			if info == nil {
				return nil
			}

			if !info.IsDir() {
				return nil
			}

			if !strings.Contains(info.Name(), "@") {
				// bar.UpdateText(pterm.FgLightMagenta.Sprintf("%v is not go.mod path,skip", info.Name()))
				return nil
			}

			modFullName := strings.TrimPrefix(osPathname, filepath.FromSlash(modRoot))
			if strings.HasPrefix(filepath.ToSlash(modFullName), "cache/") {
				return nil
			}

			modName := strings.Split(modFullName, "@")[0]
			if strings.TrimSpace(modName) == "" {
				return nil
			}

			// 如果是@v，证明还没有到最下层
			if info.Name() == "@v" {
				buf, err := utils.FileRead(filepath.Join(osPathname, "list"))
				if err != nil {
					return nil
				}

				versionList := pie.Strings(strings.Split(string(buf), "\n")).FilterNot(func(s string) bool {
					return s == ""
				}).Sort().Reverse()
				versionList.Pop()
				versionList.Each(func(s string) {
					modInfo := &ModInfo{
						FilePathList: []string{
							filepath.Join(osPathname, s+".mod"),
							filepath.Join(osPathname, s+".info"),
							filepath.Join(osPathname, s+".zip"),
							filepath.Join(osPathname, s+".lock"),
							filepath.Join(osPathname, s+".ziphash"),
						},
						Version: s,
						Name:    modName,
						ModTime: getModTime(filepath.Join(osPathname, s+".mod")),
					}

					cacheMap[modName] = append(cacheMap[modName], modInfo)

					bar.UpdateText(pterm.FgLightCyan.Sprintf("%s@%s in cache", modInfo.Name, modInfo.Version))
				})

				return filepath.SkipDir
			} else {
				modInfo := &ModInfo{
					FilePathList: []string{osPathname},
					Version:      strings.Split(modFullName, "@")[1],
					Name:         modName,
					ModTime:      getModTime(osPathname),
				}

				cacheMap[modName] = append(cacheMap[modName], modInfo)

				bar.UpdateText(pterm.FgLightCyan.Sprintf("%s@%s in cache", modInfo.Name, modInfo.Version))

				return filepath.SkipDir
			}
		},
	})
	if err != nil {
		bar.FailPrinter.Printfln("wail go mod err:%v", err)
		return
	}
	bar.Success("scanned go mod cache")

	usedMap := map[string]bool{}
	usedModMap := map[string]bool{}
	if cmdArgs.DeepClean {
		var diskList pie.Strings
		switch runtime.GOOS {
		case "windows":
			for i := 'A'; i < 'Z'; i++ {
				diskList = append(diskList, fmt.Sprintf("%c:/", i))
			}
			// diskList = append(diskList, "D:/")
		default:
			diskList = append(diskList, "/")
		}

		bar, _ := pterm.DefaultSpinner.WithText("scanning go.mod...").Start()
		var lock sync.Mutex

		add := func(name, version string) {
			bar.UpdateText(pterm.FgLightCyan.Sprintf("%s@%s in ued", name, version))
			lock.Lock()
			defer lock.Unlock()
			usedMap[fmt.Sprintf("%s@%s", name, version)] = true
			usedModMap[name] = true
		}

		skipList := pie.Strings{
			goEnv.GOMODCACHE,
			goEnv.GOROOT,
		}.FilterNot(func(s string) bool {
			return s == ""
		})

		parse := func(path string) {
			modFileBuf, err := utils.FileRead(path)
			if err != nil {
				return
			}

			mod, err := modfile.Parse(path, []byte(modFileBuf), nil)
			if err != nil {
				return
			}

			for _, value := range mod.Replace {
				add(value.New.Path, value.New.Version)
			}

			for _, value := range mod.Require {
				add(value.Mod.Path, value.Mod.Version)
			}
		}

		var pool *WorkPool
		pool = NewWorkPool(32, func(i interface{}) {
			basePath := i.(string)
			bar.UpdateText(basePath)

			if skipList.Any(func(value string) bool {
				return strings.HasPrefix(basePath, filepath.FromSlash(value))
			}) {
				return
			}

			deviceList := dirList(basePath)

			var dirList []string
			for _, device := range deviceList {
				if strings.HasSuffix(device.Path, "/go.mod") {
					parse(device.Path)
					return
				}

				if device.IsDir {
					dirList = append(dirList, device.Path)
				}
			}

			for _, dir := range dirList {
				pool.Submit(dir)
			}
		})

		diskList.Each(func(disk string) {
			pool.Submit(disk)
		})
		pool.Wait()
		bar.Success("scanned go.mod")
	}

	{
		var allRemoveCount uint32
		var allRemoveSize int64
		bar, _ := pterm.DefaultSpinner.WithText("Scanning go.mod...").Start()
		for key, values := range cacheMap {
			lastVal := &ModInfo{}

			var removeCount uint32
			var removeSize int64

			remove := func(val *ModInfo) {
				for _, file := range val.FilePathList {
					size := DirSizeB(file)

					if !cmdArgs.OnlyShow {
						err = os.RemoveAll(file)
						if err != nil {
							// pterm.Error.Printfln("err:%v", err)
							continue
						}
						bar.UpdateText(pterm.FgYellow.Sprintf("will remove %s(size:%s)", file, formatFileSize(size)))
					} else {
						bar.Success(pterm.FgYellow.Sprintf("will remove %s(size:%s)", file, formatFileSize(size)))
					}

					allRemoveCount++
					allRemoveSize += size

					removeCount++
					removeSize += size
				}
			}

			for _, val := range values {
				if usedModMap[key] {
					if lastVal.ModTime.Sub(val.ModTime) < 0 {
						if len(lastVal.FilePathList) > 0 {
							if !usedMap[fmt.Sprintf("%s@%s", lastVal.Name, lastVal.Version)] {
								remove(lastVal)
							}
						}

						lastVal = val
					} else {
						if !usedMap[fmt.Sprintf("%s@%s", val.Name, val.Version)] {
							remove(val)
						}
					}
				} else {
					remove(val)
				}
			}

			if removeCount > 0 {
				bar.Success(pterm.FgLightGreen.Sprintf("%v clean %v version size %v", key, removeCount, formatFileSize(removeSize)))
			}
		}

		if allRemoveCount > 0 {
			bar.Success(pterm.FgLightGreen.Sprintf("find %v pkg, %v version removed siz %v", len(cacheMap), allRemoveCount, formatFileSize(allRemoveSize)))
		} else {
			bar.Success(pterm.FgLightYellow.Sprintf("find %v pkg, 0 version removed", len(cacheMap)))
		}
	}
}

func formatFileSize(fileSize int64) (size string) {
	if fileSize < 1024 {
		// return strconv.FormatInt(fileSize, 10) + "B"
		return fmt.Sprintf("%.2fB", float64(fileSize)/float64(1))
	} else if fileSize < (1024 * 1024) {
		return fmt.Sprintf("%.2fKB", float64(fileSize)/float64(1024))
	} else if fileSize < (1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fMB", float64(fileSize)/float64(1024*1024))
	} else if fileSize < (1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fGB", float64(fileSize)/float64(1024*1024*1024))
	} else if fileSize < (1024 * 1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fTB", float64(fileSize)/float64(1024*1024*1024*1024))
	} else { // if fileSize < (1024 * 1024 * 1024 * 1024 * 1024 * 1024)
		return fmt.Sprintf("%.2fEB", float64(fileSize)/float64(1024*1024*1024*1024*1024))
	}
}

func DirSizeB(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if info == nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size
}

// func getFileSize(path string) int64 {
//	if !exists(path) {
//		return 0
//	}
//	fileInfo, err := os.Stat(path)
//	if err != nil {
//		return 0
//	}
//	return fileInfo.Size()
// }

func getModTime(path string) time.Time {
	if !exists(path) {
		return time.Time{}
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fileInfo.ModTime()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

func dirList(path string) []*DeviceItem {
	files, _ := ioutil.ReadDir(path)

	var deviceList []*DeviceItem
	for _, item := range files {
		if item.Name() == "" {
			continue
		}

		deviceList = append(deviceList, &DeviceItem{
			Path:  filepath.ToSlash(filepath.Join(path, item.Name())),
			IsDir: item.IsDir(),
		})
	}

	return deviceList
}
