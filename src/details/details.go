package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	addr = flag.String("addr", "localhost:9080", "the address to connect to")
)

type BookInfo struct {
	Id        int    `json:"id"`
	Author    string `json:"author"`
	Year      string `json:"year"`
	Type      string `json:"type"`
	Pages     int    `json:"pageCount"`
	Publisher string `json:"publisher"`
	Language  string `json:"language"`
	Isbn10    string `json:"ISBN-10"`
	Isbn13    string `json:"ISBN-13"`
}

type IndustryIdentifiers struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

type VolumeInfo struct {
	Language            string                `json:"language"`
	PrintType           string                `json:"printType"`
	IndustryIdentifiers []IndustryIdentifiers `json:"industryIdentifiers"`
	Authors             []string              `json:"authors"`
	PublishedDate       string                `json:"publishedDate"`
	PageCount           int                   `json:"pageCount"`
	Publisher           string                `json:"publisher"`
}
type Items struct {
	Id         string     `json:"id"`
	Etag       string     `json:"etag"`
	VolumeInfo VolumeInfo `json:"volumeInfo"`
}

type BookVolumes struct {
	Items      []Items `json:"items"`
	TotalItems int     `json:"totalItems"`
	Kind       string  `json:"kind"`
}

func main() {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		fmt.Println("health check")
		c.JSON(http.StatusOK, gin.H{
			"status": "Details is healthy",
		})
	})
	type Data struct {
		ID int `uri:"productId"`
	}
	r.GET("/details/:productId", func(c *gin.Context) {
		var data Data
		if err := c.ShouldBindUri(&data); err != nil {
			c.JSON(400, gin.H{"msg": err})
			return
		}
		id := data.ID
		headers := getForwardHeaders(c.Request)

		details, err := getBookDetails(id, headers)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"message": err.Error(),
			})
			return
		}

		var bookInfo BookInfo
		if err := json.Unmarshal([]byte(details), &bookInfo); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"message": err.Error(),
			})
			return
		} else {
			c.JSON(http.StatusOK, bookInfo)
		}
	})
	http.TimeoutHandler(r, time.Second*5, "request time out")
	log.Printf("args len: %v %s", len(os.Args), os.Args[0])
	if len(os.Args) > 1 {
		// load from Dockerfile
		if err := r.Run(fmt.Sprintf("0.0.0.0:%s", os.Args[1])); err != nil {
			log.Fatal(err)
		}
	} else {
		// for test
		if err := r.Run(fmt.Sprintf("0.0.0.0:%s", "9081")); err != nil {
			log.Fatal(err)
		}
	}
}

func getBookDetails(id int, headers map[string][]string) (string, error) {
	if os.Getenv("ENABLE_EXTERNAL_BOOK_SERVICE") == "false" {
		isbn := "0486424618"
		return fetchDetailsFromExternalService(isbn, id, headers)
	}
	var bookInfo = BookInfo{Id: id,
		Author:    "William Shakespeare",
		Year:      "1595",
		Type:      "paperback",
		Pages:     200,
		Publisher: "PublisherA",
		Language:  "English",
		Isbn10:    "1234567890",
		Isbn13:    "123-1234567890"}

	data, error := json.Marshal(&bookInfo)
	if error != nil {
		return error.Error(), error
	}
	return string(data), nil
}

func fetchDetailsFromExternalService(isbn string, id int, headers map[string][]string) (string, error) {
	//resp, err := http.Get(fmt.Sprintf("https://www.googleapis.com/books/v1/volumes?q=isbn:%s", isbn))
	//if err != nil {
	//	return err.Error(), err
	//}
	//
	//if os.Getenv("DO_NOT_ENCRYPT") == "true" {
	//	// enable ssl
	//}
	//
	//for header, value := range headers {
	//	resp.Header.Set(header, value[0])
	//}
	//
	//resp.Body.Close()
	//body, err := io.ReadAll(resp.Body)
	body, err := ioutil.ReadFile("book.json")
	if err != nil {
		return err.Error(), err
	}

	var bookVolumes BookVolumes
	if err = json.Unmarshal(body, &bookVolumes); err != nil {
		return err.Error(), err
	}

	book := bookVolumes.Items[0].VolumeInfo
	var bookType string
	if book.PrintType == "BOOK" {
		bookType = "paperback"
	} else {
		bookType = "unknown"
	}
	var language string
	if book.Language == "en" {
		language = "English"
	} else {
		language = "unknown"
	}
	isbn10 := getIsbn(book, "ISBN_10")
	isbn13 := getIsbn(book, "ISBN_13")

	var bookInfo = BookInfo{id,
		book.Authors[0],
		book.PublishedDate,
		bookType,
		book.PageCount,
		book.Publisher,
		language,
		isbn10,
		isbn13}
	var data []byte
	data, err = json.Marshal(bookInfo)
	if err != nil {
		log.Fatal(err)
		return err.Error(), err
	}
	return string(data), nil
}

func getIsbn(book VolumeInfo, isbnType string) string {
	isbnIdentifiers := book.IndustryIdentifiers
	for identifiers := range isbnIdentifiers {
		if isbnIdentifiers[identifiers].Type == isbnType {
			// do something
		}
	}
	return isbnIdentifiers[0].Identifier
}

func getForwardHeaders(request *http.Request) map[string][]string {
	headers := make(map[string][]string)
	// Keep this in sync with the headers in productpage and reviews.
	incoming_headers := []string{
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
		// Stackdriver Istio configurations.
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
	for header, value := range request.Header {
		for i := range incoming_headers {
			if header == incoming_headers[i] {
				headers[header] = value
			}
		}
	}
	return headers
}
