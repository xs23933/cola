package cola

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/schema"
	"github.com/valyala/fasthttp"
)

// maxParams defines the maximum number of parameters per route.
const maxParams = 30

// Ctx Extends fasthttp RequestCtx
type Ctx struct {
	*Core // reference to *Core
	*fasthttp.RequestCtx
	index               int
	indexHandler        int    // Index of the current handler
	method              string // HTTP method
	methodINT           int    // HTTP method INT equivalent
	path                string // HTTP path
	pathBuffer          []byte // HTTP path buffer
	detectionPath       string // Route detection path                                  -> string copy from detectionPathBuffer
	detectionPathBuffer []byte // HTTP detectionPath buffer
	pathOriginal        string
	values              [maxParams]string // Route parameter values
	treePath            string            // Path for the search in the tree
	matched             bool              // Non use route matched
	route               *Route
	baseURI             string
}

func (c *Ctx) init(ctx *fasthttp.RequestCtx) {
	c.pathOriginal = BytesToString(ctx.URI().PathOriginal())
	c.path = BytesToString(ctx.URI().Path())
	c.method = BytesToString(ctx.Request.Header.Method())
	c.methodINT = methodInt(c.method)
	c.RequestCtx = ctx

	c.index = -1
	c.indexHandler = 0
	c.matched = false
	c.baseURI = ""
	c.depPaths()
}

// Render Render View Engine
func (c *Ctx) Render(f string, optionalBind ...interface{}) error {
	var err error
	var binding interface{}
	binds := make(map[string]interface{})
	c.VisitUserValues(func(k []byte, v interface{}) {
		binds[BytesToString(k)] = v
	})

	if len(optionalBind) > 0 {
		binding = optionalBind[0]
	} else {
		binding = binds
	}

	if c.Core.Views == nil {
		err = fmt.Errorf("Render: Not Initial Views")
		Log.Error(err.Error())
		return err
	}

	c.Response.Header.SetContentType(MIMETextHTMLCharsetUTF8)
	err = c.Core.Options.Views.ExecuteWriter(c.RequestCtx.Response.BodyWriter(), f, binding)
	if err != nil {
		c.Error(err.Error(), StatusInternalServerError)
	}
	return err
}

// RenderString Parse template string to string
//
// e.g:
//   ```go
//   c.RenderString(`<div>Your price: {{ .price }}</div>`, map[string]interface{}{
//	   "price": 12.5,
//   })
//   // or use vars
//   c.Vars("price", 12.5)
//   c.RenderString(`<div>Your price: {{ .price }}</div>`)
//   ```
func (c *Ctx) RenderString(src string, optionalBind ...interface{}) (string, error) {
	tpl := template.Must(template.New("").Parse(src))
	var buf bytes.Buffer
	var binding interface{}
	binds := make(map[string]interface{})
	c.VisitUserValues(func(k []byte, v interface{}) {
		binds[BytesToString(k)] = v
	})

	if len(optionalBind) > 0 {
		binding = optionalBind[0]
	} else {
		binding = binds
	}
	err := tpl.Execute(&buf, binding)
	if err != nil {
		return "", err
	}
	return string(buf.Bytes()), err
}

// depPaths set paths for route recognition and prepared paths for the user,
// here the features for caseSensitive, decoded paths, strict paths are evaluated
func (c *Ctx) depPaths() {
	c.pathBuffer = append(c.pathBuffer[0:0], c.pathOriginal...)
	// If UnescapePath enabled, we decode the path and save it for the framework user
	if c.Core.Options.UnescapePath {
		c.pathBuffer = fasthttp.AppendUnquotedArg(c.pathBuffer[:0], c.pathBuffer)
	}
	c.path = BytesToString(c.pathBuffer)

	// another path is specified which is for routing recognition only
	// use the path that was changed by the previous configuration flags
	c.detectionPathBuffer = append(c.detectionPathBuffer[0:0], c.pathBuffer...)
	// If CaseSensitive is disabled, we lowercase the original path
	if !c.Core.CaseSensitive {
		c.detectionPathBuffer = ToLowerBytes(c.detectionPathBuffer)
	}
	// If StrictRouting is disabled, we strip all trailing slashes
	if len(c.detectionPathBuffer) > 1 && c.detectionPathBuffer[len(c.detectionPathBuffer)-1] == '/' {
		c.detectionPathBuffer = TrimRightBytes(c.detectionPathBuffer, '/')
	}
	c.detectionPath = BytesToString(c.detectionPathBuffer)

	// Define the path for dividing routes into areas for fast tree detection, so that fewer routes need to be traversed,
	// since the first three characters area select a list of routes
	c.treePath = c.treePath[0:0]
	if len(c.detectionPath) >= 3 {
		c.treePath = c.detectionPath[:3]
	}
}

// Next method in the stack that match
func (c *Ctx) Next() {
	c.indexHandler++
	if c.indexHandler < len(c.route.Handlers) {
		c.route.Handlers[c.indexHandler](c)
		return
	}
	c.Core.next(c)
}

// Append the specified value to the HTTP response header field.
// If the header is not already set, it creates the header with the specified value.
func (c *Ctx) Append(field string, values ...string) {
	if len(values) == 0 {
		return
	}
	h := BytesToString(c.Response.Header.Peek(field))
	originalH := h
	for _, value := range values {
		if len(h) == 0 {
			h = value
		} else if h != value && !strings.HasPrefix(h, value+",") && !strings.HasSuffix(h, " "+value) &&
			!strings.Contains(h, " "+value+",") {
			h += ", " + value
		}
	}
	if originalH != h {
		c.Set(field, h)
	}
}

// Get returns the HTTP request header specified by field.
// Field names are case-insensitive
// Returned value is only valid within the handler. Do not store any references.
// Make copies or use the Immutable setting instead.
func (c *Ctx) Get(key string, defaultValue ...string) string {
	return defaultString(BytesToString(c.Request.Header.Peek(key)), defaultValue)
}

// Set sets the response's HTTP header field to the specified key, value.
func (c *Ctx) Set(key string, val string) {
	c.Response.Header.Set(key, val)
}

func (c *Ctx) setCanonical(key string, val string) {
	c.Response.Header.SetCanonical(StringToBytes(key), StringToBytes(val))
}

// SendStatus sets the HTTP status code and if the response body is empty,
// it sets the correct status message in the body.
func (c *Ctx) SendStatus(status int) error {
	c.Status(status)

	// Only set status body when there is no response body
	if len(c.RequestCtx.Response.Body()) == 0 {
		return c.SendString(StatusMessage(status))
	}

	return nil
}

// SendString sets the HTTP response body for string types.
// This means no type assertion, recommended for faster performance
func (c *Ctx) SendString(body string) error {
	c.Response.SetBodyString(body)
	return nil
}

// Status sets the HTTP status for the response.
// This method is chainable.
func (c *Ctx) Status(status int) *Ctx {
	c.Response.SetStatusCode(status)
	return c
}

// Hostname contains the hostname derived from the Host HTTP header.
// Returned value is only valid within the handler. Do not store any references.
// Make copies or use the Immutable setting instead.
func (c *Ctx) Hostname() string {
	return BytesToString(c.Request.URI().Host())
}

// IP returns the remote IP address of the request.
func (c *Ctx) IP() string {
	if len(c.Core.ProxyHeader) > 0 {
		return c.Get(c.Core.ProxyHeader)
	}
	return c.RemoteIP().String()
}

// JSON converts any interface or string to JSON.
// Array and slice values encode as JSON arrays,
// except that []byte encodes as a base64-encoded string,
// and a nil slice encodes as the null JSON value.
// This method also sets the content header to application/json.
func (c *Ctx) JSON(data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	c.Response.SetBodyRaw(raw)
	c.Response.Header.SetContentType(MIMEApplicationJSON)
	return nil
}

// JSONP sends a JSON response with JSONP support.
// This method is identical to JSON, except that it opts-in to JSONP callback support.
// By default, the callback name is simply callback.
func (c *Ctx) JSONP(data interface{}, callback ...string) error {
	raw, err := json.Marshal(data)

	if err != nil {
		return err
	}

	var result, cb string

	if len(callback) > 0 {
		cb = callback[0]
	} else {
		cb = "callback"
	}

	result = cb + "(" + BytesToString(raw) + ");"

	c.setCanonical(HeaderXContentTypeOptions, "nosniff")
	c.Response.Header.SetContentType(MIMEApplicationJavaScriptCharsetUTF8)
	return c.SendString(result)
}

// ToJSON send json add status
func (c *Ctx) ToJSON(data interface{}, err error) error {
	dat := map[string]interface{}{
		"status": true,
		"msg":    "ok",
		"result": data,
	}

	if err != nil {
		dat["status"] = false
		dat["msg"] = err.Error()
	}
	return c.JSON(dat)
}

// decoderPool helps to improve ReadBody's and QueryParser's performance
var decoderPool = &sync.Pool{New: func() interface{} {
	var decoder = schema.NewDecoder()
	decoder.IgnoreUnknownKeys(true)
	return decoder
}}

// Vars makes it possible to pass interface{} values under string keys scoped to the request
// and therefore available to all following routes that match the request.
func (c *Ctx) Vars(key string, value ...interface{}) (val interface{}) {
	if len(value) == 0 {
		return c.UserValue(key)
	}
	c.SetUserValue(key, value[0])
	return value[0]
}

// MultipartForm parse form entries from binary.
// This returns a map[string][]string, so given a key the value will be a string slice.
func (c *Ctx) MultipartForm() (*multipart.Form, error) {
	return c.RequestCtx.MultipartForm()
}

// OriginalURL contains the original request URL.
// Returned value is only valid within the handler. Do not store any references.
func (c *Ctx) OriginalURL() string {
	return BytesToString(c.Request.Header.RequestURI())
}

// Params is used to get the route parameters.
// Defaults to empty string "" if the param doesn't exist.
// If a default value is given, it will return that value if the param doesn't exist.
// Returned value is only valid within the handler. Do not store any references.
// Make copies or use the Immutable setting to use the value outside the Handler.
func (c *Ctx) Params(key string, defaultValue ...string) string {
	if key == "*" || key == "+" {
		key += "1"
	}
	for i := range c.route.Params {
		if len(key) != len(c.route.Params[i]) {
			continue
		}
		if c.route.Params[i] == key {
			// in case values are not here
			if len(c.values) <= i || len(c.values[i]) == 0 {
				break
			}
			return c.values[i]
		}
	}
	return defaultString("", defaultValue)
}

// Path returns the path part of the request URL.
// Optionally, you could override the path.
func (c *Ctx) Path(override ...string) string {
	if len(override) != 0 && c.path != override[0] {
		// Set new path to context
		c.pathOriginal = override[0]

		// Set new path to request context
		c.Request.URI().SetPath(c.pathOriginal)
		// Prettify path
		c.depPaths()
	}
	return c.path
}

// SaveFile saves any multipart file to disk.
func (c *Ctx) SaveFile(fileheader *multipart.FileHeader, path string) error {
	return fasthttp.SaveMultipartFile(fileheader, path)
}

// Send sets the HTTP response body without copying it.
// From this point onward the body argument must not be changed.
func (c *Ctx) Send(body []byte) error {
	// Write response body
	c.Response.SetBodyRaw(body)
	return nil
}

var sendFileOnce sync.Once
var sendFileFS *fasthttp.FS
var sendFileHandler fasthttp.RequestHandler

// SendFile transfers the file from the given path.
// The file is not compressed by default, enable this by passing a 'true' argument
// Sets the Content-Type response HTTP header field based on the filenames extension.
func (c *Ctx) SendFile(file string, compress ...bool) error {
	// Save the filename, we will need it in the error message if the file isn't found
	filename := file

	// https://github.com/valyala/fasthttp/blob/master/fs.go#L81
	sendFileOnce.Do(func() {
		sendFileFS = &fasthttp.FS{
			Root:                 "/",
			GenerateIndexPages:   false,
			AcceptByteRange:      true,
			Compress:             true,
			CompressedFileSuffix: c.Core.Options.CompressedFileSuffix,
			CacheDuration:        10 * time.Second,
			IndexNames:           []string{"index.html"},
			PathNotFound: func(ctx *fasthttp.RequestCtx) {
				ctx.Response.SetStatusCode(StatusNotFound)
			},
		}
		sendFileHandler = sendFileFS.NewRequestHandler()
	})

	// Keep original path for mutable params
	c.pathOriginal = CopyString(c.pathOriginal)
	// Disable compression
	if len(compress) <= 0 || !compress[0] {
		// https://github.com/valyala/fasthttp/blob/master/fs.go#L46
		c.Request.Header.Del(HeaderAcceptEncoding)
	}
	// https://github.com/valyala/fasthttp/blob/master/fs.go#L85
	if len(file) == 0 || file[0] != '/' {
		hasTrailingSlash := len(file) > 0 && file[len(file)-1] == '/'
		var err error
		if file, err = filepath.Abs(file); err != nil {
			return err
		}
		if hasTrailingSlash {
			file += "/"
		}
	}
	// Set new URI for fileHandler
	c.Request.SetRequestURI(file)
	// Save status code
	status := c.Response.StatusCode()
	// Serve file
	sendFileHandler(c.RequestCtx)
	// Get the status code which is set by fasthttp
	fsStatus := c.Response.StatusCode()
	// Set the status code set by the user if it is different from the fasthttp status code and 200
	if status != fsStatus && status != StatusOK {
		c.Status(status)
	}
	// Check for error
	if status != StatusNotFound && fsStatus == StatusNotFound {
		return NewError(StatusNotFound, fmt.Sprintf("sendfile: file %s not found", filename))
	}
	return nil
}

// ContentType sets the Content-Type HTTP header(map[string] helper line 525 ) to the MIME type specified by the file extension.
func (c *Ctx) ContentType(extension string, charset ...string) *Ctx {
	if len(charset) > 0 {
		c.Response.Header.SetContentType(GetMIME(extension) + "; charset=" + charset[0])
	} else {
		c.Response.Header.SetContentType(GetMIME(extension))
	}
	return c
}

// Write appends p into response body.
func (c *Ctx) Write(p []byte) (int, error) {
	c.Response.AppendBody(p)
	return len(p), nil
}

// WriteString appends s to response body.
func (c *Ctx) WriteString(s string) (int, error) {
	c.Response.AppendBodyString(s)
	return len(s), nil
}

// Cookie data for c.Cookie
type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Path     string    `json:"path"`
	Domain   string    `json:"domain"`
	MaxAge   int       `json:"max_age"`
	Expires  time.Time `json:"expires"`
	Secure   bool      `json:"secure"`
	HTTPOnly bool      `json:"http_only"`
	SameSite string    `json:"same_site"`
}

// ClearCookie expires a specific cookie by key on the client side.
// If no key is provided it expires all cookies that came with the request.
func (c *Ctx) ClearCookie(key ...string) {
	if len(key) > 0 {
		for i := range key {
			c.Response.Header.DelClientCookie(key[i])
		}
		return
	}
	c.Request.Header.VisitAllCookie(func(k, v []byte) {
		c.Response.Header.DelClientCookieBytes(k)
	})
}

// ClearCookies clear cookie set default path /
//   if ClearCookie failed.
// If no key is provided it expires all cookies that came with the request.
func (c *Ctx) ClearCookies(key ...string) {
	if len(key) > 0 {
		for i := range key {
			c.DelCookie(key[i], "/")
		}
		return
	}
	c.Request.Header.VisitAllCookie(func(k, v []byte) {
		c.Response.Header.DelClientCookieBytes(k)
	})
}

// DelCookie delete cookie with path
func (c *Ctx) DelCookie(k string, path ...string) {
	c.Response.Header.DelClientCookie(k)
	c.Response.Header.DelCookie(k)
	fc := fasthttp.AcquireCookie()
	fc.SetKey(k)
	fc.SetExpire(fasthttp.CookieExpireDelete)
	if len(path) > 0 {
		fc.SetPath(path[0])
	}
	c.Response.Header.SetCookie(fc)
	fasthttp.ReleaseCookie(fc)
}

// Cookie sets a cookie by passing a cookie struct.
func (c *Ctx) Cookie(cookie *Cookie) {
	fc := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(fc)
	fc.SetKey(cookie.Name)
	fc.SetValue(cookie.Value)
	fc.SetPath(cookie.Path)
	fc.SetDomain(cookie.Domain)
	fc.SetMaxAge(cookie.MaxAge)
	fc.SetExpire(cookie.Expires)
	fc.SetSecure(cookie.Secure)
	fc.SetHTTPOnly(cookie.HTTPOnly)

	switch ToLower(cookie.SameSite) {
	case "strict":
		fc.SetSameSite(fasthttp.CookieSameSiteStrictMode)
	case "none":
		fc.SetSameSite(fasthttp.CookieSameSiteNoneMode)
	default:
		fc.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	}

	c.Response.Header.SetCookie(fc)
}

// Cookies is used for getting a cookie value by key.
// Defaults to the empty string "" if the cookie doesn't exist.
// If a default value is given, it will return that value if the cookie doesn't exist.
// The returned value is only valid within the handler. Do not store any references.
// Make copies or use the Immutable setting to use the value outside the Handler.
func (c *Ctx) Cookies(key string, defaultValue ...string) string {
	return defaultString(BytesToString(c.Request.Header.Cookie(key)), defaultValue)
}

// Body contains the raw body submitted in a POST request.
// Returned value is only valid within the handler. Do not store any references.
// Make copies or use the Immutable setting instead.
func (c *Ctx) Body() []byte {
	return c.Request.Body()
}

// ReadBody binds the request body to a struct.
// It supports decoding the following content types based on the Content-Type header:
// application/json, application/xml, application/x-www-form-urlencoded, multipart/form-data
// If none of the content types above are matched, it will return a ErrUnprocessableEntity error
func (c *Ctx) ReadBody(out interface{}) error {
	// Get decoder from pool
	schemaDecoder := decoderPool.Get().(*schema.Decoder)
	defer decoderPool.Put(schemaDecoder)

	// Get content-type
	ctype := ToLower(BytesToString(c.Request.Header.ContentType()))

	switch {
	case strings.HasPrefix(ctype, MIMEApplicationJSON):
		schemaDecoder.SetAliasTag("json")
		return json.Unmarshal(c.Request.Body(), out)
	case strings.HasPrefix(ctype, MIMEApplicationForm):
		schemaDecoder.SetAliasTag("form")
		data := make(map[string][]string)
		c.PostArgs().VisitAll(func(key []byte, val []byte) {
			data[BytesToString(key)] = append(data[BytesToString(key)], BytesToString(val))
		})
		return schemaDecoder.Decode(out, data)
	case strings.HasPrefix(ctype, MIMEMultipartForm):
		schemaDecoder.SetAliasTag("form")
		data, err := c.MultipartForm()
		if err != nil {
			return err
		}
		return schemaDecoder.Decode(out, data.Value)
	case strings.HasPrefix(ctype, MIMETextXML), strings.HasPrefix(ctype, MIMEApplicationXML):
		schemaDecoder.SetAliasTag("xml")
		return xml.Unmarshal(c.Request.Body(), out)
	}
	// No suitable content type found
	return ErrUnprocessableEntity
}

// FormFile returns the first file by key from a MultipartForm.
func (c *Ctx) FormFile(key string) (*multipart.FileHeader, error) {
	return c.RequestCtx.FormFile(key)
}

// FormValue returns the first value by key from a MultipartForm.
// Defaults to the empty string "" if the form value doesn't exist.
// If a default value is given, it will return that value if the form value does not exist.
// Returned value is only valid within the handler. Do not store any references.
// Make copies or use the Immutable setting instead.
func (c *Ctx) FormValue(key string, defaultValue ...string) string {
	return defaultString(BytesToString(c.RequestCtx.FormValue(key)), defaultValue)
}

// Fresh returns true when the response is still “fresh” in the client's cache,
// otherwise false is returned to indicate that the client cache is now stale
// and the full response should be sent.
// When a client sends the Cache-Control: no-cache request header to indicate an end-to-end
// reload request, this module will return false to make handling these requests transparent.
// https://github.com/jshttp/fresh/blob/10e0471669dbbfbfd8de65bc6efac2ddd0bfa057/index.js#L33
func (c *Ctx) Fresh() bool {
	// fields
	var modifiedSince = c.Get(HeaderIfModifiedSince)
	var noneMatch = c.Get(HeaderIfNoneMatch)

	// unconditional request
	if modifiedSince == "" && noneMatch == "" {
		return false
	}

	// Always return stale when Cache-Control: no-cache
	// to support end-to-end reload requests
	// https://tools.ietf.org/html/rfc2616#section-14.9.4
	cacheControl := c.Get(HeaderCacheControl)
	if cacheControl != "" && isNoCache(cacheControl) {
		return false
	}

	// if-none-match
	if noneMatch != "" && noneMatch != "*" {
		var etag = BytesToString(c.Response.Header.Peek(HeaderETag))
		if etag == "" {
			return false
		}
		if isEtagStale(etag, StringToBytes(noneMatch)) {
			return false
		}

		if modifiedSince != "" {
			var lastModified = BytesToString(c.Response.Header.Peek(HeaderLastModified))
			if lastModified != "" {
				lastModifiedTime, err := http.ParseTime(lastModified)
				if err != nil {
					return false
				}
				modifiedSinceTime, err := http.ParseTime(modifiedSince)
				if err != nil {
					return false
				}
				return lastModifiedTime.Before(modifiedSinceTime)
			}
		}
	}
	return true
}
