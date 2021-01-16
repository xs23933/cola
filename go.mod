module github.com/xs23933/cola

go 1.15

require (
	github.com/andybalholm/brotli v1.0.1 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/gorilla/schema v1.2.0
	github.com/klauspost/compress v1.11.6 // indirect
	github.com/valyala/fasthttp v1.19.0
	github.com/xs23933/cola-model v0.0.0-00010101000000-000000000000
	golang.org/x/sys v0.0.0-20210113181707-4bcb84eeeb78 // indirect
	gorm.io/gorm v1.20.11
)

replace github.com/xs23933/cola-model => ../cola-model
