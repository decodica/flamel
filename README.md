# Flamel: framework for Google App Engine

Flamel is a session-less, simple web framework built to structure and ease development of web apps running on Google App Engine using the Go API.

It exposes a minimalistic lifecycle and implements its own performant and low-allocation routing system.

Flamel comes with a fully fledged and optional ORM-like layer (the model package) over the GCP Datastore, Search and Memcache APIs to help keep quotas low while increasing productivity.
CORS and AMP requests are also supported. 

Except for the Google Client Libraries and the Appengine Package (for obvious reasons), Flamel comes with zero dependencies.

Flamel is 100% compatible with GAE go1.11 API

- Fast and reliable (simple requests are responded to in less than 10000 ns)
- Not invasive: its own packages are all optional, including the router, which means you can build your own system of top of flamel lifecycle:
as an example, you can mix and match the model package with the official client libraries, or just use the latter.
Mage is built on top of few interfaces: you can use the stock implementations or you can roll your own renderer, use your own router, build your own controller.
- Squared and structured: the lifecycle and the basic interfaces the framework is composed of allow client code to be simple and well organized. The architecture of Flamel 
incentives the use of stateless logic and helps in implementing proper http responses by not hiding the protocol logic.

## Installation

```bash
go get -u github.com/decodica/flamel
```

## Usage

- [Quickstart]

To start using some alchemy goodies, set up the `app.yaml` file as you would for any GAE go application:

```yaml
runtime: go111

handlers:

- url: /.*
  script: auto
```

Next we need to define our Flamel Application and a controller, by implementing the following interfaces:

The first one is the `Application` interface:
```go
type Application interface {
	OnStart(ctx context.Context) context.Context
	AfterResponse(ctx context.Context)
}
```

`OnStart()` will be called for each request that Flamel will handle, as soon as an appengine context is available and *before* any routing happens.
`AfterResponse()` is instead called *after* the response has been delivered.

As a minimum, we can implement the following `Application`:

```go
type HelloWorld struct {}

func (app HelloWorld) OnStart(ctx context.Context) context.Context {
    return ctx
}

func (app HelloWorld) AfterResponse(ctx context.Context) {} 
```

The second interface is the `Controller` interface:

```go
type Controller interface {
	Process(ctx context.Context, out *ResponseOutput) HttpResponse
	OnDestroy(ctx context.Context)
}
```

`OnDestroy()` will be invoked after the response output has been sent to the client, but before the application `AfterResponse()` method is invoked.
`Process()` is the heart of the controller. It is here that the response logic must be implemented, for any request that our application wants to handle.

The following controller responds with the usual "Hello World!" and returns a 405 status code if the request method is not "GET": 

```go
type HelloWorldController struct {}

func (controller *HelloWorldController) Process(ctx context.Context, out *flamel.ResponseOutput) flamel.HttpResponse {

	ins := flamel.InputsFromContext(ctx)
	method := ins[flamel.KeyRequestMethod].Value()
	switch method {
	case http.MethodGet:
		renderer := flamel.TextRenderer{}
		renderer.Data = "Hello Flamel!"
		out.Renderer = &renderer
		return flamel.HttpResponse{Status:http.StatusOK}
	}
	
	return flamel.HttpResponse{Status:http.StatusMethodNotAllowed}
}

func (controller *HelloWorldController) OnDestroy(ctx context.Context) {}
```

Putting all the pieces together allows us to have a GAE application up and running (and well organized):

```go
package main

import "context"
import "decodica.com/flamel"
import "net/http"

// Define the application struct

type HelloWorld struct {}

func (app HelloWorld) OnStart(ctx context.Context) context.Context {
    return ctx
}

func (app HelloWorld) AfterResponse(ctx context.Context) {}

// Define our one and only controller

type HelloWorldController struct {}

func (controller *HelloWorldController) Process(ctx context.Context, out *flamel.ResponseOutput) flamel.HttpResponse {

	ins := flamel.InputsFromContext(ctx)
	method := ins[flamel.KeyRequestMethod].Value()
	switch method {
	case http.MethodGet:
		renderer := flamel.TextRenderer{}
		renderer.Data = "Hello Flamel!"
		out.Renderer = &renderer
		return flamel.HttpResponse{Status:http.StatusOK}
	}
	
	return flamel.HttpResponse{Status:http.StatusMethodNotAllowed}
}

func (controller *HelloWorldController) OnDestroy(ctx context.Context) {}

func main() {
    instance := flamel.Instance()
    instance.SetRoute("/", func(ctx context.Context) flamel.Controller {
            c := HelloWorldController{}
            return &c
        }, nil)
    
    app := HelloWorld{}
    instance.Run(app)
}
```

Now running
```bash 
dev_appserver.py app.yaml
```

and sending a GET request to `localhost:8080` will make your app output `Hello Flamel!` 

- [The lifecycle]
// Todo

- [Handling routes]
// Todo

- [Managing authentication]
// Todo

- [The data layer: the model package]
// Todo

## Roadmap

- Support for GAE go112 API by replacing Memcache and Search with Redis and Elastic on the GCP (or any other official solution Google will provide us with - Memcached API perhaps?).
- Rewriting of the `model` package using `unsafe` in place of `reflect`. This should be fun!

Any help would be greatly appreciated. :)

## License

Flamel uses the [MIT](LICENSE.TXT) license.
