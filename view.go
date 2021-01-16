package cola

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Views view interface
type Views interface {
	Theme(string)
	Load() error
	ExecuteWriter(io.Writer, string, interface{}, ...string) error
}

// ViewEngine html template engine
type ViewEngine struct {
	left      string // default {{
	right     string // default }}
	directory string
	theme     string // use theme folder
	// layout variable name that incapsulates the template
	layout     string
	layoutFunc string
	// determines if the engine parsed all templates
	loaded bool
	// reload on each render
	reload bool
	// views extension
	ext string
	// debug prints the parsed templates
	debug bool
	// lock for funcmap and templates
	mutex sync.RWMutex
	// template funcmap
	helpers template.FuncMap
	// templates
	Templates *template.Template
	// http.FileSystem supports embedded files
	fileSystem http.FileSystem
}

// NewView Create view engine
// args:
//  string theme
//  bool debug
//  map[string]interface{} helper fn
//  http.FileSystem file system
func NewView(directory, ext string, args ...interface{}) *ViewEngine {
	engine := &ViewEngine{
		left: "{{", right: "}}",
		directory:  directory,
		ext:        ext,
		layoutFunc: "yield",
		helpers:    templateHelpers,
	}

	for _, arg := range args {
		switch a := arg.(type) {
		case string: // string define theme
			engine.theme = a
		case bool: // bool is debug
			engine.debug = a
		case map[string]interface{}:
			for k, fn := range a {
				engine.helpers[k] = fn
			}
		case http.FileSystem:
			engine.fileSystem = a
		}
	}

	if engine.debug {
		engine.reload = true
	}
	engine.AddFunc(engine.layoutFunc, func() error {
		return fmt.Errorf("layout called unexpectedly")
	})
	return engine
}

// AddFunc add helper func
func (ve *ViewEngine) AddFunc(name string, fn interface{}) *ViewEngine {
	ve.mutex.Lock()
	defer ve.mutex.Unlock()
	ve.helpers[name] = fn
	return ve
}

// Reload if set to true the templates are reloading on each render,
// use it when you're in development and you don't want to restart
func (ve *ViewEngine) Reload(reload bool) *ViewEngine {
	ve.reload = reload
	return ve
}

// Delims sets the action delimiters to the specified strings
// Default: {{ var }}
func (ve *ViewEngine) Delims(l, r string) *ViewEngine {
	ve.left, ve.right = l, r
	return ve
}

// Theme sets theme
func (ve *ViewEngine) Theme(theme string) {
	ve.theme = theme
	ve.loaded = false
}

// Load load tmpl file
func (ve *ViewEngine) Load() error {
	if ve.loaded {
		return nil
	}
	ve.mutex.Lock()
	defer ve.mutex.Unlock()
	ve.Templates = template.New(ve.directory)

	ve.Templates.Delims(ve.left, ve.right)
	ve.Templates.Funcs(ve.helpers)

	directory := ve.directory
	if ve.theme != "" { // just load theme sub folder
		directory = filepath.Join(ve.directory, ve.theme)
	}

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil { // Return error if exist
			return err
		}
		if info == nil || info.IsDir() { // Skip file if it's a directory or has no file info
			return nil
		}
		ext := filepath.Ext(path) // get file ext of file
		if ext != ve.ext {
			return nil
		}

		rel, err := filepath.Rel(directory, path) // get the relative file path
		if err != nil {
			return err
		}

		name := filepath.ToSlash(rel)           // Reverse slashes '\' -> '/' and e.g part\head.html -> part/head.html
		name = strings.TrimSuffix(name, ve.ext) // Remove ext from name 'index.html' -> 'index'

		buf, err := ReadFile(path, ve.fileSystem)
		if err != nil {
			return err
		}

		// Create new template associated with the current one
		// This enable use to invoke other templates {{ template .. }}
		_, err = ve.Templates.New(name).Parse(string(buf))
		if err != nil {
			return err
		}
		if err != nil {
			return err
		}

		if ve.debug {
			Log.Debug("Views: parsed template: %s\n", name)
		}
		return err
	}

	ve.loaded = true
	if ve.fileSystem != nil {
		return Walk(ve.fileSystem, directory, walkFn)
	}

	return filepath.Walk(directory, walkFn)
}

// Layout set layout file Layout
func (ve *ViewEngine) Layout(layout string) *ViewEngine {
	ve.layout = layout
	return ve
}

// ExecuteWriter execute render
func (ve *ViewEngine) ExecuteWriter(out io.Writer, tpl string, binding interface{}, layout ...string) error {
	if !ve.loaded || ve.reload {
		if ve.reload {
			ve.loaded = false
		}
		if err := ve.Load(); err != nil {
			return err
		}
	}

	tmpl := ve.Templates.Lookup(tpl)
	if tmpl == nil {
		return fmt.Errorf("render: template %s does not exist", tpl)
	}
	layoutTpl := ve.layout
	if len(layout) > 0 {
		layoutTpl = layout[0]
	}
	if len(layoutTpl) > 0 {
		lay := ve.Templates.Lookup(layoutTpl)
		if lay == nil {
			return fmt.Errorf("render: layout %s does not exist", layoutTpl)
		}
		lay.Funcs(map[string]interface{}{
			ve.layoutFunc: func() error {
				return tmpl.Execute(out, binding)
			},
		})
		return lay.Execute(out, binding)
	}
	return tmpl.Execute(out, binding)
}

var templateHelpers = template.FuncMap{
	// Replaces newlines with <br>
	"nl2br": func(text string) template.HTML {
		return template.HTML(strings.Replace(template.HTMLEscapeString(text), "\n", "<br>", -1))
	},
	// Skips sanitation on the parameter.  Do not use with dynamic data.
	"raw": func(text string) template.HTML {
		return template.HTML(text)
	},
	// Format a date according to the application's default date(time) format.
	"date": func(date time.Time, f ...string) string {
		df := DefaultDateFormat
		if len(f) > 0 {
			df = f[0]
		}
		return date.Format(df)
	},
	// datetime format a datetime
	"datetime": func(date time.Time, f ...string) string {
		df := DefaultDateTimeFormat
		if len(f) > 0 {
			df = f[0]
		}
		return date.Format(df)
	},
}

// Revel's default date and time constants
const (
	DefaultDateFormat     = "2006-01-02"
	DefaultDateTimeFormat = "2006-01-02 15:04"
)
