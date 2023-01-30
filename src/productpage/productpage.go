package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type Data struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Children []Data `json:"children"`
}

type Details struct {
	Id        int    `json:"id"`
	Author    string `json:"author"`
	Year      string `json:"year"`
	Type      string `json:"type"`
	Pages     int    `json:"pageCount"`
	Publisher string `json:"publisher"`
	Language  string `json:"language"`
	Isbn10    string `json:"ISBN-10"`
	Isbn13    string `json:"ISBN-13"`
	Error     string `json:"error"`
}

type Rating struct {
	Stars int    `json:"stars"`
	Color string `json:"color"`
	Error string `json:"error"`
}

type Reviewer struct {
	Reviewer string `json:"reviewer"`
	Text     string `json:"text"`
	Rating   Rating `json:"rating"`
}

type Reviewers struct {
	Id          int        `json:"id"`
	PodName     string     `json:"podname"`
	ClusterName string     `json:"clusterame"`
	Reviewers   []Reviewer `json:"reviewers"`
	Error       string     `json:"error"`
}

type Product struct {
	ID              int    `json:"productId"`
	Title           string `json:"title"`
	DescriptionHtml string `json:"descriptionHtml"`
}

var servicesDomain string
var detailsHostname string
var ratingsHostname string
var reviewsHostname string

var productPage Data
var details Data
var reviews Data
var ratings Data

var floodFactor int

func init() {
	value, ok := os.LookupEnv("SERVICES_DOMAIN")
	if !ok {
		servicesDomain = ""
	} else {
		servicesDomain = value
	}
	value, ok = os.LookupEnv("DETAILS_HOSTNAME")
	if !ok {
		//detailsHostname = "details"
		detailsHostname = "127.0.0.1"
	} else {
		detailsHostname = value
	}
	value, ok = os.LookupEnv("RATINGS_HOSTNAME")
	if !ok {
		//ratingsHostname = "ratings"
		ratingsHostname = "127.0.0.1"
	} else {
		ratingsHostname = value
	}
	value, ok = os.LookupEnv("REVIEWS_HOSTNAME")
	if !ok {
		//reviewsHostname = "reviews"
		reviewsHostname = "127.0.0.1"
	} else {
		reviewsHostname = value
	}
	value, ok = os.LookupEnv("FLOOD_FACTOR")
	if !ok {
		floodFactor = 0
	} else {
		floodFactor, _ = strconv.Atoi(value)
	}

	ratings = Data{
		Name:     fmt.Sprintf("http://%s%s:9080", ratingsHostname, servicesDomain),
		Endpoint: "ratings",
		Children: []Data{},
	}

	details = Data{
		Name:     fmt.Sprintf("http://%s%s:9081", detailsHostname, servicesDomain),
		Endpoint: "details",
		Children: []Data{},
	}

	reviews = Data{
		Name:     fmt.Sprintf("http://%s%s:9082", reviewsHostname, servicesDomain),
		Endpoint: "reviews",
		Children: []Data{ratings},
	}

	productPage = Data{
		Name:     fmt.Sprintf("http://%s%s:9080", detailsHostname, servicesDomain),
		Endpoint: "details",
		Children: []Data{details, reviews},
	}
}

func main() {
	r := gin.Default()

	rateLoop := func(n int) []struct{} {
		return make([]struct{}, n)
	}
	reduce := func(n int, m int) int {
		return n - m
	}
	htmlRender := func(n string) template.HTML {
		return template.HTML(n)
	}
	r.SetFuncMap(template.FuncMap{
		"rateLoop":   rateLoop,
		"reduce":     reduce,
		"htmlRender": htmlRender,
	})

	r.Static("static", "static")

	r.LoadHTMLGlob("templates/*")

	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))

	r.GET("/health", func(c *gin.Context) {
		fmt.Println("health check")
		c.JSON(http.StatusOK, gin.H{
			"status": "Product page is healthy",
		})
	})

	var indexHandle = func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", productPage)
	}
	r.GET("/", indexHandle)
	r.GET("/index.html", indexHandle)

	r.POST("/login", func(c *gin.Context) {
		user := c.Request.FormValue("username")
		c.Redirect(http.StatusMovedPermanently, c.Request.Referer())

		log.Printf("referer: %s\n", c.Request.Referer())

		session := sessions.Default(c)
		session.Set("user", user)
		session.Save()
		//c.JSON(http.StatusOK, gin.H{
		//	"status": "login success",
		//})
		c.Next()
	})

	r.GET("/logout", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, c.Request.Referer())
		session := sessions.Default(c)
		session.Set("user", nil)
		c.JSON(http.StatusOK, gin.H{
			"status": "logout success",
		})
	})

	r.GET("/productpage", func(c *gin.Context) {
		var productId = 0 // TODO: replace default value
		headers := getForwardHeaders(c)

		session := sessions.Default(c)
		user := session.Get("user")
		log.Printf("productpage user : %s\n", user)
		product := getProduct(productId)
		detailsStatus, detailsStr := getProductDetails(productId, headers)

		log.Print("detail:", detailsStr)

		var details Details
		var err = json.Unmarshal([]byte(detailsStr), &details)
		if err != nil {
			log.Fatal("Unmarshal error", err)
		}

		if floodFactor > 0 {
			floodReviews(productId, headers)
		}

		reviewsStatus, reviewsStr := getProductReviews(productId, headers)

		log.Print("review:", reviewsStr)

		var reviews Reviewers
		json.Unmarshal([]byte(reviewsStr), &reviews)
		if err != nil {
			log.Fatal(err)
		}

		type Result struct {
			DetailsStatus int         `json:"detailsStatus"`
			ReviewsStatus int         `json:"reviewsStatus"`
			Product       Product     `json:"product"`
			Details       Details     `json:"details"`
			Reviews       Reviewers   `json:"reviews"`
			User          interface{} `json:"user"`
		}
		var result = Result{DetailsStatus: detailsStatus,
			ReviewsStatus: reviewsStatus,
			Product:       product,
			Details:       details,
			Reviews:       reviews,
			User:          user}
		d, err := json.Marshal(result)
		log.Print("d:", string(d))
		c.HTML(http.StatusOK, "productpage.html", result)
	})

	// The API:
	r.GET("/api/v1/products", productRoute)
	r.GET("/api/v1/products/<product_id>", productRoutes)
	r.GET("/api/v1/products/<product_id>/reviews>", reviewsRoute)
	r.GET("/api/v1/products/<product_id>/ratings", ratingsRoute)

	if len(os.Args) < 2 {
		err := r.Run("0.0.0.0:9080")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		p := os.Args[1]
		log.Printf("start at port %v\n", p)
		// Make it compatible with IPv6 if Linux
		err := r.Run(fmt.Sprintf("0.0.0.0:%s", p))
		if err != nil {
			log.Fatal(err)
		}
	}
}

func ratingsRoute(c *gin.Context) {
	productId, _ := strconv.Atoi(c.Query("product_id"))
	headers := getForwardHeaders(c)
	status, ratings := getProductRatings(productId, headers)
	c.JSON(status, ratings)
}

func reviewsRoute(c *gin.Context) {
	productId, _ := strconv.Atoi(c.Query("product_id"))
	headers := getForwardHeaders(c)
	status, reviews := getProductReviews(productId, headers)
	c.JSON(status, reviews)
}

func productRoute(c *gin.Context) {
	c.Redirect(http.StatusMovedPermanently, c.Request.Referer())
	session := sessions.Default(c)
	session.Set("user", nil)
	c.JSON(http.StatusOK, getProducts())
}

func productRoutes(c *gin.Context) {
	productId, _ := strconv.Atoi(c.Query("product_id"))
	headers := getForwardHeaders(c)
	status, details := getProductDetails(productId, headers)
	c.JSON(status, details)
}

func getProductRatings(productId int, headers map[string][]string) (statusCode int, respStr string) {
	client := http.Client{
		Timeout: 3 * time.Second,
	}
	request, err := http.NewRequest("GET", fmt.Sprintf("%s/%s/%v", ratings.Name, ratings.Endpoint, productId), nil)

	for header, value := range headers {
		request.Header.Set(header, value[0])
	}

	resp, err := client.Do(request)

	if err != nil {
		log.Fatal(err)
		return http.StatusInternalServerError, "{'error': 'request ratings unavailable!.'}"
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
		// try again
	} else {
		return http.StatusOK, string(body)
	}
	return http.StatusInternalServerError, "{'error': 'Sorry, product ratings are currently unavailable for this book.'}"
}

func getProductReviews(productId int, headers map[string][]string) (statusCode int, respStr string) {
	// Do not remove. Bug introduced explicitly for illustration in fault injection task
	// TODO: Figure out how to achieve the same effect using Envoy retries/timeouts

	for i := 0; i < 2; i++ {
		client := http.Client{
			Timeout: 3 * time.Second,
		}
		request, err := http.NewRequest("GET", fmt.Sprintf("%s/%s/%v", reviews.Name, reviews.Endpoint, productId), nil)

		for header, value := range headers {
			request.Header.Set(header, value[0])
		}

		resp, err := client.Do(request)

		if err != nil {
			log.Println("err:", err)
			return http.StatusInternalServerError, "{'error': 'request reviews unavailable!.'}"
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
			// try again
		} else {
			return http.StatusOK, string(body)
		}
	}
	return http.StatusInternalServerError, "{'error': 'Sorry, product reviews are currently unavailable for this book.'}"

}

func getProductReviewsIgnoreResponse(productId int, headers map[string][]string) {
	getProductReviews(productId, headers)
}

func floodReviewsAsynchronously(productId int, headers map[string][]string) {
	// the response is disregarded
	//await asyncio.gather(*(getProductReviewsIgnoreResponse(product_id, headers) for _ in range(flood_factor)))
	for i := 0; i < floodFactor; i++ {
		go getProductReviewsIgnoreResponse(productId, headers)
	}
}

func floodReviews(productId int, headers map[string][]string) {
	//loop = asyncio.new_event_loop()
	//loop.run_until_complete(floodReviewsAsynchronously(product_id, headers))
	//loop.close()
	go floodReviewsAsynchronously(productId, headers)
}

func getProducts() []Product {
	return []Product{{ID: 0, Title: "The Comedy of Errors", DescriptionHtml: "<a href='https://en.wikipedia.org/wiki/The_Comedy_of_Errors'>Wikipedia Summary</a>: The Comedy of Errors is one of <b>William Shakespeare's</b> early plays. It is his shortest and one of his most farcical comedies, with a major part of the humour coming from slapstick and mistaken identity, in addition to puns and word play."}}
}

func getProduct(productId int) Product {
	products := getProducts()
	if productId+1 > len(products) {
		return Product{}
	} else {
		return products[productId]
	}
}

func getForwardHeaders(c *gin.Context) map[string][]string {
	headers := make(map[string][]string)

	//TODO uncomment
	// x-b3-*** headers can be populated using the opentracing span
	//span := get_current_span()
	//carrier := make(map[string][]string)
	//tracer.inject(
	//span_context=span.context,
	//format=Format.HTTP_HEADERS,
	//carrier=carrier)

	//headers.update(carrier)

	// We handle other (non x-b3-***) headers manually

	session := sessions.Default(c)
	user := session.Get("user")
	log.Printf("getForwardHeaders user: %s\n", user)
	if user != nil {
		headers["end-user"] = []string{fmt.Sprint(user)}
	}

	// Keep this in sync with the headers in details and reviews.
	incomingHeaders := []string{
		// All applications should propagate x-request-id. This header is
		// included in access log statements and is used for consistent trace
		// sampling and log sampling decisions in Istio.
		"x-request-id",

		// Lightstep tracing header. Propagate this if you use lightstep tracing
		// in Istio (see
		// https://istio.io/latest/docs/tasks/observability/distributed-tracing/lightstep/)
		// Note: this should probably be changed to use B3 or W3C TRACE_CONTEXT.
		// Lightstep recommends using B3 or TRACE_CONTEXT and most application
		// libraries from lightstep do not support x-ot-span-context.
		"x-ot-span-context",

		// Datadog tracing header. Propagate these headers if you use Datadog
		// tracing.
		"x-datadog-trace-id",
		"x-datadog-parent-id",
		"x-datadog-sampling-priority",

		// W3C Trace Context. Compatible with OpenCensusAgent and Stackdriver Istio
		// configurations.
		"traceparent",
		"tracestate",

		// Cloud trace context. Compatible with OpenCensusAgent and Stackdriver Istio
		// configurations.
		"x-cloud-trace-context",

		// Grpc binary trace context. Compatible with OpenCensusAgent nad
		// Stackdriver Istio configurations.
		"grpc-trace-bin",

		// b3 trace headers. Compatible with Zipkin, OpenCensusAgent, and
		// Stackdriver Istio configurations. Commented out since they are
		// propagated by the OpenTracing tracer above.
		// 'x-b3-traceid',
		// 'x-b3-spanid',
		// 'x-b3-parentspanid',
		// 'x-b3-sampled',
		// 'x-b3-flags',

		// Application-specific headers to forward.
		"user-agent",

		// Context and session specific headers
		"cookie",
		"authorization",
		"jwt",
	}
	// For Zipkin, always propagate b3 headers.
	// For Lightstep, always propagate the x-ot-span-context header.
	// For Datadog, propagate the corresponding datadog headers.
	// For OpenCensusAgent and Stackdriver configurations, you can choose any
	// set of compatible headers to propagate within your application. For
	// example, you can propagate b3 headers or W3C trace context headers with
	// the same result. This can also allow you to translate between context
	// propagation mechanisms between different applications.
	for header, value := range c.Request.Header {
		for i := range incomingHeaders {
			if header == incomingHeaders[i] {
				headers[header] = value
			}
		}
	}

	return headers
}

func getProductDetails(productId int, headers map[string][]string) (statsCode int, respStr string) {
	client := http.Client{
		Timeout: 3 * time.Second,
	}
	url := fmt.Sprintf("%s/%s/%v", details.Name, details.Endpoint, productId)
	log.Println("url:", url)
	request, err := http.NewRequest("GET", url, nil)

	for header, value := range headers {
		request.Header.Set(header, value[0])
	}
	resp, err := client.Do(request)

	if err != nil {
		log.Println("err:", err)
		return http.StatusInternalServerError, "{\"error\": \"request unavailable!.\",}"
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("get product", err)
		return http.StatusInternalServerError, "\"error\": \"Sorry, product details are currently unavailable for this book.\""
	} else {
		log.Print("return ", string(body))
		return http.StatusOK, string(body)
	}
}
