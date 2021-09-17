package main

import (
	"fmt"
	"github.com/Luoxin/Eutamias/utils"
	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/eddieivan01/nic"
	"github.com/gookit/color"
	"github.com/rogpeppe/go-internal/modfile"
	log "github.com/sirupsen/logrus"
	"net/http"
	"path"
	"runtime"
	"time"
)

func main() {
	log.SetFormatter(&nested.Formatter{
		FieldsOrder: []string{
			log.FieldKeyTime, log.FieldKeyLevel, log.FieldKeyFile,
			log.FieldKeyFunc, log.FieldKeyMsg,
		},
		CustomCallerFormatter: func(f *runtime.Frame) string {
			return fmt.Sprintf("(%s %s:%d)", f.Function, path.Base(f.File), f.Line)
		},
		TimestampFormat:  time.RFC3339,
		HideKeys:         true,
		NoFieldsSpace:    true,
		NoUppercaseLevel: true,
		TrimMessages:     true,
		CallerFirst:      true,
	})
	_ = UpdateAllMod("D:\\develop\\Eutamias\\go.mod")
}

func UpdateAllMod(modePath string) error {
	type ModVersionRsp struct {
		Version string    `json:"Version"`
		Time    time.Time `json:"Time"`
	}

	modFileBuf, err := utils.FileRead(modePath)
	if err != nil {
		log.Errorf("err:%v", err)
		return err
	}

	mod, err := modfile.Parse(modePath, []byte(modFileBuf), nil)
	if err != nil {
		log.Errorf("err:%v", err)
		return err
	}

	if mod.Go.Version < "1.17" {
		log.Warnf("mod version under then 1.17")
	}

	var changed bool
	replaceMap := map[string]bool{}
	for _, replace := range mod.Replace {
		if replace.Syntax == nil {
			continue
		}

		if len(replace.Syntax.Token) == 0 {
			continue
		}

		line := replace.Syntax.Token[0]
		replaceMap[line] = true
	}

	for _, require := range mod.Require {
		if replaceMap[require.Mod.Path] {
			continue
		}

		var resp *nic.Response
		resp, err = nic.Get(fmt.Sprintf("https://goproxy.cn/%s/@latest", require.Mod.Path), nic.H{
			Auth:          nil,
			AllowRedirect: true,
			Timeout:       5,
			Chunked:       true,
			SkipVerifyTLS: true,
		})
		if err != nil {
			log.Errorf("err:%v", err)
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			var rsp ModVersionRsp
			err = resp.JSON(&rsp)
			if err != nil {
				log.Errorf("err:%v", err)
				log.Errorf(resp.Text)
				continue
			}

			if require.Mod.Version != rsp.Version {
				log.Infof("update %v from %v to %v",
					color.Cyan.Text(require.Mod.Path), color.Yellow.Text(require.Mod.Version), color.Magenta.Text(rsp.Version))
				err = mod.AddRequire(require.Mod.Path, rsp.Version)
				if err != nil {
					log.Errorf("err:%v", err)
					continue
				}

				changed = true
			} else {
				log.Infof("%v is already latest", color.Cyan.Text(require.Mod.Path))
			}
		case http.StatusNotFound:
			log.Warnf("%v is not found", color.Cyan.Text(require.Mod.Path))
		default:
			log.Warnf("not support:%v", resp.StatusCode)
		}

	}

	if changed {
		buf, err := mod.Format()
		if err != nil {
			log.Errorf("err:%v", err)
			return err
		}

		err = utils.FileWrite(modePath, string(buf))
		if err != nil {
			log.Errorf("err:%v", err)
			return err
		}
	}

	return nil
}
