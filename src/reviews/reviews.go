package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type Reviewer struct {
	Reviewer1 int `json:"Reviewer1"`
	Reviewer2 int `json:"Reviewer2"`
}
type Result struct {
	Id      int      `json:"id"`
	Ratings Reviewer `json:"ratings"`
}

var ratingsEnabled bool
var starColor string
var servicesDomain string
var ratingsHostname string
var ratingsService string
var podHostname string
var clusterName string

// HTTP headers to propagate for distributed tracing are documented at
// https://istio.io/docs/tasks/telemetry/distributed-tracing/overview/#trace-context-propagation
var headersToPropagate = []string{
	// All applications should propagate x-request-id. This header is
	// included in access log statements and is used for consistent trace
	// sampling and log sampling decisions in Istio.
	"x-request-id",

	// Lightstep tracing header. Propagate this if you use lightstep tracing
	// in Istio (see
	// https://istio.io/latest/docs/tasks/observability/distributed-tracing/lightstep/)
	// Note:this should probably be changed to use B3 or W3C TRACE_CONTEXT.
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
	"x-b3-traceid",
	"x-b3-spanid",
	"x-b3-parentspanid",
	"x-b3-sampled",
	"x-b3-flags",

	// Application-specific headers to forward.
	"end-user",
	"user-agent",

	// Context and session specific headers
	"cookie",
	"authorization",
	"jwt",
}

func init() {
	var value, ok = os.LookupEnv("ENABLE_RATINGS")
	if !ok {
		ratingsEnabled = false
	} else {
		ratingsEnabled, _ = strconv.ParseBool(value)
	}
	value, ok = os.LookupEnv("STAR_COLOR")
	if !ok {
		starColor = "black"
	} else {
		starColor = value
	}

	value, ok = os.LookupEnv("SERVICES_DOMAIN")
	if !ok {
		servicesDomain = ""
	} else {
		servicesDomain = fmt.Sprintf(".%s", value)
	}

	value, ok = os.LookupEnv("RATINGS_HOSTNAME")
	if !ok {
		ratingsHostname = "ratings"
	} else {
		ratingsHostname = fmt.Sprintf(".%s", value)
	}
	ratingsService = fmt.Sprintf("http://%s%s:9080/ratings", ratingsHostname, servicesDomain)

	podHostname = os.Getenv("HOSTNAME")
	clusterName = os.Getenv("CLUSTER_NAME")
}

func main() {
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
	})

	r.GET("/health", func(c *gin.Context) {
		fmt.Println("health check")
		c.JSON(http.StatusOK, gin.H{
			"status": "Reviews is healthy",
		})
	})

	type Data struct {
		ID int `uri:"productId"`
	}

	r.GET("/reviews/:productId", func(c *gin.Context) {

		starsReviewer1 := -1
		starsReviewer2 := -1

		var data Data
		if err := c.ShouldBindUri(&data); err != nil {
			c.JSON(400, gin.H{"msg": err})
			return
		}
		productId := data.ID

		if ratingsEnabled {
			ratings := getRatings(productId, c.Request.Header)
			starsReviewer1 = ratings.Ratings.Reviewer1
			starsReviewer2 = ratings.Ratings.Reviewer2
		}

		jsonResStr := getJsonResponse(productId, starsReviewer1, starsReviewer2)

		c.JSON(http.StatusOK, jsonResStr)
	})
	if len(os.Args) > 1 {
		// load from Dockerfile
		if err := r.Run(fmt.Sprintf("0.0.0.0:%s", os.Args[1])); err != nil {
			log.Fatal(err)
		}
	} else {
		// for test
		if err := r.Run(fmt.Sprintf("0.0.0.0:%s", "9082")); err != nil {
			log.Fatal(err)
		}
	}
}

func getJsonResponse(productId int, starsReviewer1 int, starsReviewer2 int) interface{} {

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

	type Result struct {
		Id          int        `json:"id"`
		PodName     string     `json:"podname"`
		ClusterName string     `json:"clusterame"`
		Reviewers   []Reviewer `json:"reviewers"`
	}

	var rat1 Rating
	if ratingsEnabled {
		if starsReviewer1 != -1 {
			rat1 = Rating{Stars: starsReviewer1, Color: starColor}
		} else {
			rat1 = Rating{Error: "Ratings service is currently unavailable"}
		}
	}
	var rat2 Rating
	if ratingsEnabled {
		if starsReviewer2 != -1 {
			rat2 = Rating{Stars: starsReviewer2, Color: starColor}
		} else {
			rat2 = Rating{Error: "Ratings service is currently unavailable"}
		}
	}
	var r = Result{
		Id: productId, PodName: podHostname,
		ClusterName: clusterName,
		Reviewers: []Reviewer{{Reviewer: "Reviewer1", Text: "An extremely entertaining play by Shakespeare. The slapstick humour is refreshing!", Rating: rat1},
			{Reviewer: "Reviewer2", Text: "Absolutely fun and entertaining. The play lacks thematic depth when compared to other plays by Shakespeare.", Rating: rat2}},
	}

	return r
}

func getRatings(productId int, headers map[string][]string) Result {
	////cb.property("com.ibm.ws.jaxrs.client.connection.timeout", timeout)
	////cb.property("com.ibm.ws.jaxrs.client.receive.timeout", timeout)
	var timeout time.Duration
	if starColor == "black" {
		timeout = 10000
	} else {
		timeout = 2500
	}
	client := http.Client{
		Timeout: timeout * time.Second,
	}
	request, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", ratingsService, productId), nil)
	if err != nil {
		log.Fatal(err)
	}
	for i := range headersToPropagate {
		value := headers[headersToPropagate[i]]
		request.Header.Set(headersToPropagate[i], value[0])
	}

	resp, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	statusCode := resp.StatusCode
	if statusCode != http.StatusOK {
		log.Printf("Error:unable to contact %s got status of %v", ratingsService, statusCode)
		return Result{}
	} else {
		var data []byte
		_, err := resp.Body.Read(data)
		if err != nil {
			log.Fatal(err)
		}
		var result Result
		err = json.Unmarshal(data, &result)
		if err != nil {
			log.Fatal(err)
		}
		return result
	}
}
