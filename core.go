package cola

import (
	"cola/log"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/reuseport"
	model "github.com/xs23933/cola-model"
	"gorm.io/gorm"
)

// Hand Handler
type Hand func(*Ctx)

// Db db config
type Db struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

// Options Global options
type Options struct {
	Prefork bool

	*Db `yaml:"db"`

	LogPath string

	// Debug Default false
	Debug bool

	Views Views

	// Case sensitive routing, all to lowercase
	CaseSensitive bool

	// Nginx Caddy some proxy header X-Real-IP  X-Forwarded-For
	// Default: ""
	ProxyHeader string
	// Server: cola
	ServerName string
	// Default: false
	ETag bool `json:"etag"`
	// Max body size that the server accepts.
	// -1 will decline any body size
	//
	// Default: 4 * 1024 * 1024
	BodyLimit int `json:"body_limit"`
	// Maximum number of concurrent connections.
	//
	// Default: 256 * 1024
	Concurrency int `json:"concurrency"`

	// When set to true, converts all encoded characters in the route back
	// before setting the path for the context, so that the routing,
	// the returning of the current url from the context `ctx.Path()`
	// and the paramters `ctx.Params(%key%)` with decoded characters will work
	//
	// Default: false
	UnescapePath bool `json:"unescape_path"`

	// The amount of time allowed to read the full request including body.
	// It is reset after the request handler has returned.
	// The connection's read deadline is reset when the connection opens.
	//
	// Default: unlimited
	ReadTimeout time.Duration `json:"read_timeout"`

	// The maximum duration before timing out writes of the response.
	// It is reset after the request handler has returned.
	//
	// Default: unlimited
	WriteTimeout time.Duration `json:"write_timeout"`

	// The maximum amount of time to wait for the next request when keep-alive is enabled.
	// If IdleTimeout is zero, the value of ReadTimeout is used.
	//
	// Default: unlimited
	IdleTimeout time.Duration `json:"idle_timeout"`

	// Per-connection buffer size for requests' reading.
	// This also limits the maximum header size.
	// Increase this buffer if your clients send multi-KB RequestURIs
	// and/or multi-KB headers (for example, BIG cookies).
	//
	// Default: 4096
	ReadBufferSize int `json:"read_buffer_size"`

	// Per-connection buffer size for responses' writing.
	//
	// Default: 4096
	WriteBufferSize int `json:"write_buffer_size"`

	// CompressedFileSuffix adds suffix to the original file name and
	// tries saving the resulting compressed file under the new file name.
	//
	// Default: ".gz"
	CompressedFileSuffix string `json:"compressed_file_suffix"`
}

// Core cola core
type Core struct {
	*Options
	*fasthttp.Server
	pool sync.Pool
	// Route stack divided by HTTP methods
	stack [][]*Route
	// Route stack divided by HTTP methods and route prefixes
	treeStack []map[string][]*Route
	mutex     sync.Mutex
	// Amount of registered routes
	routesCount int
}

// Serve start cola
//
// addr 0.0.0.0:8080
//      8080
//      :8080
func (c *Core) Serve(args ...interface{}) error {
	var (
		addr = "8080"
		ln   net.Listener
		err  error
		tc   *tls.Config
	)

	for _, arg := range args {
		switch a := arg.(type) {
		case int:
			addr = strconv.Itoa(a)
		case string:
			addr = a
		case *tls.Config:
			tc = a
		}
	}

	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	if c.Prefork {
		return c.prefork(addr, tc)
	}

	if ln, err = net.Listen("tcp", addr); err != nil {
		return err
	}

	if tc != nil {
		ln = tls.NewListener(ln, tc)
	}
	return c.Server.Serve(ln)
}

// Use register a other plugin middware module TODO:
func (c *Core) Use(args ...interface{}) *Core {
	path := ""
	var handlers []Hand
	skip := false
	for _, arg := range args {
		switch a := arg.(type) {
		case string:
			path = a
		case Module:
			RegisterModule(a)
		case Hand:
			handlers = append(handlers, a)
		case handle:
			skip = true
			c.buildHandles(a)
		}
	}
	if skip {
		return c
	}

	c.pushMethod("USE", path, handlers...)
	return c
}

func (c *Core) buildHandles(h handle) {
	h.Init() // call init
	// register routers
	refCtl := reflect.TypeOf(h)
	methodCount := refCtl.NumMethod()
	valFn := reflect.ValueOf(h)
	prefix := h.Prefix()
	c.pushMethod(methodUse, prefix, h.Preload) // Register Global preload
	for i := 0; i < methodCount; i++ {
		m := refCtl.Method(i)
		name := toNamer(m.Name)
		if fn, ok := (valFn.Method(i).Interface()).(func(*Ctx)); ok {
			for _, method := range Methods {
				if strings.HasPrefix(name, ToLower(method)) {
					name = fixURI(prefix, name, method)
					c.pushMethod(method, name, fn)
				}
			}
		}
	}
}

// Add add some method.
func (c *Core) Add(method, path string, handlers ...Hand) {
	c.pushMethod(method, path, handlers...)
}

func (c *Core) pushMethod(method, pathRaw string, handlers ...Hand) *Core {
	method = ToUpper(method)
	if method != methodUse && methodInt(method) == -1 {
		panic(fmt.Sprintf("pushMethod: invalid http method %s\n", method))
	}

	// A route requires atleast one ctx handler
	if len(handlers) == 0 {
		panic(fmt.Sprintf("missing handler in route: %s\n", pathRaw))
	}

	if pathRaw == "" {
		pathRaw = "/"
	}

	if pathRaw[0] != '/' {
		pathRaw = "/" + pathRaw
	}

	pathPretty := pathRaw

	if !c.CaseSensitive && len(pathPretty) > 1 {
		pathPretty = ToLower(pathPretty)
	}

	if len(pathPretty) > 1 {
		pathPretty = TrimRight(pathPretty, '/')
	}

	isUse := method == methodUse
	isStar := pathPretty == "/*"
	isRoot := pathPretty == "/"

	parsedRaw := parseRoute(pathRaw)
	parsedPretty := parseRoute(pathPretty)

	route := Route{
		use:  isUse,
		star: isStar,
		root: isRoot,

		path:        pathPretty,
		routeParser: parsedPretty,
		Params:      parsedRaw.params,

		Path:     pathRaw,
		Method:   method,
		Handlers: handlers,
	}

	Log.Debug("%s: %s", method, route.Path)

	if isUse {
		for _, m := range Methods {
			c.addRoute(m, &route)
		}
		return c
	}
	c.addRoute(method, &route)
	return nil
}
func (c *Core) pushStatic(prefix, root string, config ...Static) *Core {
	// For security we want to restrict to the current work directory.
	if len(root) == 0 {
		root = "."
	}
	// Cannot have an empty prefix
	if prefix == "" {
		prefix = "/"
	}
	// Prefix always start with a '/' or '*'
	if prefix[0] != '/' {
		prefix = "/" + prefix
	}
	// in case sensitive routing, all to lowercase
	if !c.CaseSensitive {
		prefix = ToLower(prefix)
	}
	// Strip trailing slashes from the root path
	if len(root) > 0 && root[len(root)-1] == '/' {
		root = root[:len(root)-1]
	}
	// Is prefix a direct wildcard?
	var isStar = prefix == "/*"
	// Is prefix a root slash?
	var isRoot = prefix == "/"
	// Is prefix a partial wildcard?
	if strings.Contains(prefix, "*") {
		// /john* -> /john
		isStar = true
		prefix = strings.Split(prefix, "*")[0]
		// Fix this later
	}
	prefixLen := len(prefix)
	// Fileserver settings
	fs := &fasthttp.FS{
		Root:                 root,
		GenerateIndexPages:   false,
		AcceptByteRange:      false,
		Compress:             false,
		CompressedFileSuffix: c.CompressedFileSuffix,
		CacheDuration:        10 * time.Second,
		IndexNames:           []string{"index.html"},
		PathRewrite: func(fctx *fasthttp.RequestCtx) []byte {
			path := fctx.Path()
			if len(path) >= prefixLen {
				if isStar && BytesToString(path[0:prefixLen]) == prefix {
					path = append(path[0:0], '/')
				} else if len(path) > 0 && path[len(path)-1] != '/' {
					path = append(path[prefixLen:], '/')
				}
			}
			if len(path) > 0 && path[0] != '/' {
				path = append([]byte("/"), path...)
			}
			return path
		},
		PathNotFound: func(fctx *fasthttp.RequestCtx) {
			fctx.Response.SetStatusCode(StatusNotFound)
		},
	}

	// Set config if provided
	var cacheControlValue string
	if len(config) > 0 {
		maxAge := config[0].MaxAge
		if maxAge > 0 {
			cacheControlValue = "public, max-age=" + strconv.Itoa(maxAge)
		}
		fs.CacheDuration = config[0].CacheDuration
		fs.Compress = config[0].Compress
		fs.AcceptByteRange = config[0].ByteRange
		fs.GenerateIndexPages = config[0].Browse
		if config[0].Index != "" {
			fs.IndexNames = []string{config[0].Index}
		}
	}
	fileHandler := fs.NewRequestHandler()
	handler := func(c *Ctx) {
		// Serve file
		fileHandler(c.RequestCtx)
		// Return request if found and not forbidden
		status := c.Response.StatusCode()
		if status != StatusNotFound && status != StatusForbidden {
			if len(cacheControlValue) > 0 {
				c.Response.Header.Set(HeaderCacheControl, cacheControlValue)
			}
			return
		}
		// Reset response to default
		c.SetContentType("") // Issue #420
		c.Response.SetStatusCode(StatusOK)
		c.Response.SetBodyString("")
		c.Next()
	}

	// Create route metadata without pointer
	route := Route{
		// Router booleans
		use:  true,
		root: isRoot,
		path: prefix,
		// Public data
		Method:   MethodGet,
		Path:     prefix,
		Handlers: []Hand{handler},
	}
	// Add route to stack
	c.addRoute(MethodGet, &route)
	// Add HEAD route
	c.addRoute(MethodHead, &route)
	return c
}

func (c *Core) addRoute(method string, route *Route) {
	// Get unique HTTP method indentifier
	m := methodInt(method)

	// prevent identically route registration
	l := len(c.stack[m])
	if l > 0 && c.stack[m][l-1].Path == route.Path && route.use == c.stack[m][l-1].use {
		preRoute := c.stack[m][l-1]
		preRoute.Handlers = append(preRoute.Handlers, route.Handlers...)
	} else {
		// Increment global route position
		c.mutex.Lock()
		c.routesCount++
		c.mutex.Unlock()
		route.pos = c.routesCount
		route.Method = method
		// Add route to the stack
		c.stack[m] = append(c.stack[m], route)
	}
	// Build router tree
	c.buildTree()
}

func (c *Core) buildTree() *Core {
	// loop all the methods and stacks and create the prefix tree
	for m := range Methods {
		c.treeStack[m] = make(map[string][]*Route)
		for _, route := range c.stack[m] {
			treePath := ""
			if len(route.routeParser.segs) > 0 && len(route.routeParser.segs[0].Const) >= 3 {
				treePath = route.routeParser.segs[0].Const[:3]
			}
			// create tree stack
			c.treeStack[m][treePath] = append(c.treeStack[m][treePath], route)
		}
	}
	// loop the methods and tree stacks and add global stack and sort everything
	for m := range Methods {
		for treePart := range c.treeStack[m] {
			if treePart != "" {
				// merge global tree routes in current tree stack
				c.treeStack[m][treePart] = uniqueRouteStack(append(c.treeStack[m][treePart], c.treeStack[m][""]...))
			}
			// sort tree slices with the positions
			sort.Slice(c.treeStack[m][treePart], func(i, j int) bool {
				return c.treeStack[m][treePart][i].pos < c.treeStack[m][treePart][j].pos
			})
		}
	}

	return c
}
func (c *Core) prefork(addr string, tlsConfig *tls.Config) (err error) {
	if isChild() {
		runtime.GOMAXPROCS(1)
		var ln net.Listener
		if ln, err = reuseport.Listen("tcp4", addr); err != nil {
			return err
		}

		if tlsConfig != nil {
			ln = tls.NewListener(ln, tlsConfig)
		}

		Go(watchMaster)

		return c.Server.Serve(ln)
	}

	type child struct {
		pid int
		err error
	}

	var max = runtime.GOMAXPROCS(0)
	var childs = make(map[int]*exec.Cmd)
	var channel = make(chan child, max)

	defer func() { // defer kill all childs process
		for _, proc := range childs {
			_ = proc.Process.Kill()
		}
	}()

	var pids []string

	for i := 0; i < max; i++ {
		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", envChildKey, envChildVal))

		if err = cmd.Start(); err != nil {
			return fmt.Errorf("failed to start a child prefork process, error: %v", err)
		}

		pid := cmd.Process.Pid
		childs[pid] = cmd
		pids = append(pids, strconv.Itoa(pid))

		go func() {
			channel <- child{pid, cmd.Wait()}
		}()
	}

	return (<-channel).err
}

func (c *Core) init() {
	if c.Options == nil {
		c.Options = &Options{}
	}
	if c.Options.BodyLimit <= 0 {
		c.Options.BodyLimit = 4 * 1024 * 1024
	}
	if c.Options.Concurrency <= 0 {
		c.Options.Concurrency = 256 * 1024
	}
	if c.Options.ReadBufferSize <= 0 {
		c.Options.ReadBufferSize = 4096
	}
	if c.Options.WriteBufferSize <= 0 {
		c.Options.WriteBufferSize = 4096
	}

	if c.Options.ServerName == "" {
		c.Options.ServerName = "Cola"
	}

	if c.Options.CompressedFileSuffix == "" {
		c.CompressedFileSuffix = ".gz"
	}

	logLevel := log.LevelWarn
	if c.Options.Debug {
		logLevel = log.LevelDebug
	}
	logOutput := os.Stdout
	if c.Options.LogPath != "" {
		fileName := filepath.Base(os.Args[0])
		logOutput, _ = os.OpenFile(filepath.Join(c.Options.LogPath, fileName+".log"), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0755)
	}

	Log = log.NewLogger(logOutput, logLevel)

	if c.Options.Db.Driver != "" && c.Options.Db.DSN != "" {
		db, err := model.New(c.Options.Db.Driver, c.Options.Db.DSN, c.Options.Debug)
		if err != nil {
			Log.Error(err.Error())
			os.Exit(1)
		}
		DB = db
	}
	if c.Options.Views != nil {
		if err := c.Views.Load(); err != nil {
			p, _ := filepath.Abs("./view")
			Log.Debug("Views: %v\n", p)
			Log.Error("Views: %v\n", err)
		}
	}

	c.Server = &fasthttp.Server{
		Logger:             Log,
		Handler:            c.handleRequest,
		Name:               c.ServerName,
		Concurrency:        c.Options.Concurrency,
		ReadTimeout:        c.Options.ReadTimeout,
		WriteTimeout:       c.Options.WriteTimeout,
		IdleTimeout:        c.Options.IdleTimeout,
		ReadBufferSize:     c.Options.ReadBufferSize,
		WriteBufferSize:    c.Options.WriteBufferSize,
		MaxRequestBodySize: c.Options.BodyLimit,
	}
}

func (c *Core) next(ctx *Ctx) (match bool, err error) {
	// Get stack length
	tree, ok := c.treeStack[ctx.methodINT][ctx.treePath]
	if !ok {
		tree = c.treeStack[ctx.methodINT][""]
	}
	lenr := len(tree) - 1

	// Loop over the route stack starting from previous index
	for ctx.index < lenr {
		// Increment route index
		ctx.index++

		// Get *Route
		route := tree[ctx.index]

		// Check if it matches the request path
		match = route.match(ctx.detectionPath, ctx.path, &ctx.values)

		// No match, next route
		if !match {
			continue
		}
		// Pass route reference and param values
		ctx.route = route

		// Non use handler matched
		if !ctx.matched && !route.use {
			ctx.matched = true
		}

		// Execute first handler of route
		ctx.indexHandler = 0
		route.Handlers[0](ctx)
		return match, nil // Stop scanning the stack
	}

	// If ctx.Next() does not match, return 404
	_ = ctx.SendStatus(StatusNotFound)
	_ = ctx.SendString("Cannot " + ctx.method + " " + ctx.pathOriginal)

	if !ctx.matched && methodExist(ctx) {
		err = ErrMethodNotAllowed
	}
	return
}

// handleRequest All request processing center
func (c *Core) handleRequest(fctx *fasthttp.RequestCtx) {
	ctx := c.assignCtx(fctx)
	defer c.releaseCtx(ctx)
	if ctx.methodINT == -1 {
		ctx.Status(StatusBadRequest).SendString("Invalid http method")
		return
	}

	// Delegate next to handle the request
	// Find match in stack
	match, err := c.next(ctx)
	if err != nil {
		_ = ctx.SendStatus(StatusInternalServerError)
	}
	// Generate ETag if enabled
	if match && c.ETag {
		setETag(ctx, false)
	}
}

// like https://github.com/qiangxue/fasthttp-routing/blob/master/router.go
func (c *Core) assignCtx(fctx *fasthttp.RequestCtx) *Ctx {
	ctx := c.pool.Get().(*Ctx)
	ctx.Core = c
	ctx.init(fctx)
	return ctx
}

func (c *Core) releaseCtx(ctx *Ctx) {
	// ctx.Route = nil
	ctx.RequestCtx = nil
	c.pool.Put(ctx)
}

// New Create cola web instance.
func New(opts ...interface{}) *Core {
	methodsLen := len(Methods)
	c := &Core{
		// Create router stack
		stack:     make([][]*Route, methodsLen),
		treeStack: make([]map[string][]*Route, methodsLen),
		pool: sync.Pool{
			New: func() interface{} {
				return new(Ctx)
			},
		},
	}
	for _, opt := range opts {
		switch v := opt.(type) {
		case *Options:
			c.Options = v
		}
	}

	c.init()
	return c
}

func isChild() bool {
	return os.Getenv(envChildKey) == envChildVal
}

func watchMaster() {
	if runtime.GOOS == "windows" {
		p, err := os.FindProcess(os.Getppid())
		if err == nil {
			_, _ = p.Wait()
		}
		os.Exit(1)
	}
	// if it is equal to 1 (init process ID),
	// it indicates that the master process has exited
	for range time.NewTicker(time.Millisecond * 500).C {
		if os.Getppid() == 1 {
			os.Exit(1)
		}
	}
}

// Go starts a recoverable goroutine.
func Go(goroutine func()) {
	GoWithRecover(goroutine, defaultRecoverGoroutine)
}

// GoWithRecover starts a recoverable goroutine using given customRecover() function.
func GoWithRecover(goroutine func(), customRecover func(err interface{})) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				customRecover(err)
			}
		}()
		goroutine()
	}()
}

func defaultRecoverGoroutine(err interface{}) {
	Log.Error("Error in Go routine: %s", err)
	Log.Error("Stack: %s", debug.Stack())
}

const (
	envChildKey = "COLA_CHILD"
	envChildVal = "1"
)

var (
	// Log default global log interface
	Log log.Interface
	// Config Global config
	Config map[string]interface{}
)
