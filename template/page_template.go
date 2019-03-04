package template

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Template is a structure that serves as a web page build.
type Template struct {
	Config        *TemplateConfig
	tileMapping   map[string]string
	cacheFile     map[string]*cacheFile
	cacheTemplate map[string]*cacheTemplate
	controller    map[string]interface{}
}

type cacheFile struct {
	modUnixNano int64
	file        []byte
}

type cacheTemplate struct {
	modUnixNano int64
	template    map[string]interface{}
}

// NewTemplate is creator of the Template
func NewTemplate(TemplateConfig *TemplateConfig) *Template {
	if TemplateConfig.WelcomeFile == "" {
		TemplateConfig.WelcomeFile = "index"
	}
	p := &Template{
		Config:        TemplateConfig,
		tileMapping:   map[string]string{},
		cacheFile:     map[string]*cacheFile{},
		cacheTemplate: map[string]*cacheTemplate{},
		controller:    map[string]interface{}{},
	}
	return p
}

// AddController is add page controller
func (p *Template) AddController(prefix string, con interface{}) {
	if prefix == "/" || prefix == "" {
		p.controller[""] = con
		p.controller["/"] = con
	} else {
		p.controller[prefix] = con
	}
}

// SetTile determines page tile according to urlpath
func (p *Template) SetTile(urlPath string, tileSet string) {
	p.tileMapping[urlPath] = tileSet
}

// Route is the role of distributing pages according to url
func (p *Template) Route(r *http.Request, urlpath string) (data []byte, err error) {
	purePath := regexp.MustCompile("[?#:;]").Split(urlpath, -1)[0]
	paths := strings.Split(purePath, "/")
	if paths[len(paths)-1] == "" {
		paths[len(paths)-1] = p.Config.WelcomeFile
		purePath = strings.Join(paths, "/")
	}

	tileSet := "default"

	for i := 0; i < len(paths); i++ {
		path := strings.Join(paths[:i+1], "/")
		if tileSetTemp, has := p.tileMapping[path]; has {
			tileSet = tileSetTemp
		}
	}

	template, err := p.loadTemplate(p.Config.TemplatePath + "/" + purePath + ".html")
	if err != nil {
		merge := map[string]interface{}{}
		err2 := p.controllerBuild(purePath, r, func(m map[string]interface{}) error {
			for key, bs := range m {
				merge[key] = bs
			}
			return nil
		})
		if err2 != nil {
			return nil, err2
		}
		if len(merge) > 0 {
			l, err := json.Marshal(merge)
			if err != nil {
				return nil, err
			}
			return l, nil
		}
		return nil, err
	}

	// tile load
	data, err = p.loadFile(p.Config.LayoutPath + "/" + tileSet + ".html")
	if err != nil {
		return nil, err
	}

	// file build
	data, err = p.pageFileBuilder(data)
	if err != nil {
		return nil, err
	}
	// template build
	data, err = p.pageBuilder(data, template)
	if err != nil {
		return nil, err
	}
	// file build
	data, err = p.pageFileBuilder(data)
	if err != nil {
		return nil, err
	}

	err = p.controllerBuild(purePath, r, func(m map[string]interface{}) error {
		data, err = p.pageBuilder(data, m)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	data = p.pageCleaner(data)

	return
}

func (p *Template) controllerBuild(purePath string, r *http.Request, data func(map[string]interface{}) error) error {
	// controller build
	sPath := strings.Split(purePath, "/")
	controllMethod := sPath[len(sPath)-1]
	for i, conName := range sPath { // 각 depth 컨트롤러를 처리
		if i == len(sPath)-1 {
			conName = "/"
		}
		con := p.controller[conName]
		method, err := getMethod(con, controllMethod)
		if err != nil {
			continue
		}
		m, err := callMethod(method, r)
		if err != nil {
			return err
		}

		err = data(m)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Template) loadTemplate(path string) (m map[string]interface{}, err error) {
	var info os.FileInfo
	info, err = os.Stat(path)
	if err != nil {
		return
	}
	ct, has := p.cacheTemplate[path]
	if has && ct.modUnixNano == info.ModTime().UnixNano() {
		m = ct.template
		return
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}

	bf := bytes.NewReader(data)
	n, err := html.Parse(bf)
	if err != nil {
		return
	}
	h, err := getTag(n, "head")
	if err != nil {
		return
	}
	b, err := getTag(n, "body")
	if err != nil {
		return
	}

	m = map[string]interface{}{}
	extractPage(h.FirstChild, m)
	extractPage(b.FirstChild, m)
	p.cacheTemplate[path] = &cacheTemplate{
		modUnixNano: info.ModTime().UnixNano(),
		template:    m,
	}

	return
}

func (p *Template) loadFile(path string) (data []byte, err error) {
	var info os.FileInfo
	info, err = os.Stat(path)
	if err != nil {
		return
	}
	cf, has := p.cacheFile[path]
	if has && cf.modUnixNano == info.ModTime().UnixNano() {
		data = cf.file
		return
	}

	data, err = ioutil.ReadFile(path)
	if err != nil {
		return
	}
	p.cacheFile[path] = &cacheFile{
		modUnixNano: info.ModTime().UnixNano(),
		file:        data,
	}
	return
}

func (p *Template) pageCleaner(data []byte) []byte {
	var validID = regexp.MustCompile(`{{(.*?)}}`)
	return validID.ReplaceAll(data, []byte{})
}

func (p *Template) pageBuilder(data []byte, m map[string]interface{}) ([]byte, error) {
	var validID = regexp.MustCompile(`{{(.*?)}}`)
	return validID.ReplaceAllFunc(data, func(b []byte) []byte {
		key := string(b[2 : len(b)-2])
		if v, has := m[key]; has {
			if bs, ok := v.([]byte); ok {
				return bs
			}
		}
		key = strings.ToLower(key)
		if v, has := m[key]; has {
			if bs, ok := v.([]byte); ok {
				return bs
			}
		}
		return b
	}), nil
	// return []byte(htmlStr), nil
}

func (p *Template) pageFileBuilder(data []byte) ([]byte, error) {
	var validID = regexp.MustCompile(`{{(.*?)}}`)
	return validID.ReplaceAllFunc(data, func(b []byte) []byte {
		fileName := string(b[2 : len(b)-2])

		file, err := p.loadFile(p.Config.LayoutPath + "/" + fileName + ".html")
		if err != nil {
			return b
		}
		return file
	}), nil
	// return []byte(htmlStr), nil
}
