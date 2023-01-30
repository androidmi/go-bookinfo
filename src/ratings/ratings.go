package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var healthy = true
var unavailable = false
var userAddedRatings []Result // used to demonstrate POST functionality

var db *sql.DB

var hostName string
var portNumber string
var username string
var password string
var url string

type Reviewer struct {
	Reviewer1 int `json:"Reviewer1"`
	Reviewer2 int `json:"Reviewer2"`
}
type Result struct {
	Id      int      `json:"id"`
	Ratings Reviewer `json:"ratings"`
}

type Ratings struct {
	Rating int `json:"rating"`
}

func init() {
	if os.Getenv("SERVICE_VERSION") == "v-unhealthy" {
		// make the service unavailable once in 15 minutes for 15 minutes.
		// 15 minutes is chosen since the Kubernetes's exponential back-off is reset after 10 minutes
		// of successful execution
		// see https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy
		// Kiali shows the last 10 or 30 minutes, so to show the error rate of 50%,
		// it will be required to run the service for 30 minutes, 15 minutes of each state (healthy/unhealthy)
		//setInterval(function () {
		//	healthy = !healthy
		//	unavailable = !unavailable
		//}, 900000);
		t := time.Tick(time.Second * 900)
		go func(h bool, ua bool) {
			for {
				<-t
				healthy = !h
				unavailable = !ua
			}
		}(healthy, unavailable)
	}
	if os.Getenv("SERVICE_VERSION") == "v-unavailable" {
		// make the service unavailable once in 60 seconds
		//setInterval(function () {
		//	unavailable = !unavailable
		//}, 60000);

		t := time.Tick(time.Second * 60)
		go func(ua bool) {
			for {
				<-t
				unavailable = !ua
			}
		}(unavailable)
	}

	/**
	 * We default to using mongodb, if DB_TYPE is not set to mysql.
	 */
	if os.Getenv("SERVICE_VERSION") == "v2" {
		if os.Getenv("DB_TYPE") == "mysql" {
			hostName = os.Getenv("MYSQL_DB_HOST")
			portNumber = os.Getenv("MYSQL_DB_PORT")
			username = os.Getenv("MYSQL_DB_USER")
			password = os.Getenv("MYSQL_DB_PASSWORD")
		} else {
			url = os.Getenv("MONGO_DB_URL")
		}
	}
}

func main() {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		fmt.Println("health check")
		if healthy {
			c.JSON(http.StatusOK, gin.H{
				"status": "Ratings is healthy",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "Ratings is not healthy",
			})
		}

	})
	r.POST("/^\\/ratings\\/[0-9]*/", func(c *gin.Context) {
		req := c.Request
		productIdStr := strings.Split(req.URL.Path, "/")[0]
		productId, err := strconv.Atoi(productIdStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "please provide numeric product ID",
			})
			log.Fatal(err)
			return
		} else {
			var body []byte
			_, err := req.Body.Read(body)
			if err != nil {
				log.Fatal(err)
			}
			var reviewer Reviewer
			err = json.Unmarshal([]byte(body), &reviewer)
			if err != nil {
				log.Fatal(err)
			}
			c.JSON(http.StatusBadRequest, putLocalReviews(productId, reviewer))
		}
	})
	r.GET("/^\\/ratings\\/[0-9]*/", func(c *gin.Context) {
		req := c.Request
		productIdStr := strings.Split(req.URL.Path, "/")[0]
		productId, err := strconv.Atoi(productIdStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "please provide numeric product ID",
			})
			log.Fatal(err)
			return
		} else if os.Getenv("SERVICE_VERSION") == "v2" {
			var firstRating = 0
			var secondRating = 0
			if os.Getenv("DB_TYPE") == "mysql" {
				//"root:password@tcp(127.0.0.1:3306)/mysqldb?charset=utf8"
				dbUrl := fmt.Sprintf("%s:%s@tcp(%s:%s)/mysqldb?charset=utf8", username, password, hostName, portNumber)
				db, err = sql.Open("mysql", dbUrl)
				defer func(db *sql.DB) {
					err := db.Close()
					if err != nil {
						log.Fatal("db close error", err)
					}
				}(db)
				db.SetMaxOpenConns(2000)
				db.SetMaxIdleConns(1000)
				db.SetMaxOpenConns(10)
				if err := db.Ping(); err != nil {
					fmt.Println(err)
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "open mysql database fail",
					})
					return
				}
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "could not connect to ratings database",
					})
					log.Fatal(err)
					return
				}
				rows, err := db.Query("SELECT Rating FROM ratings")
				defer func(rows *sql.Rows) {
					err := rows.Close()
					if err != nil {
						log.Fatal("rows close error", err)
					}
				}(rows)
				if err != nil {
					fmt.Println("query error")
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "could not perform select",
					})
					return
				}

				var ratings Ratings
				err = rows.Scan(ratings.Rating)
				firstRating = ratings.Rating
				rows.Next()

				err = rows.Scan(ratings.Rating)
				secondRating = ratings.Rating

				var result = Result{Id: productId, Ratings: Reviewer{firstRating, secondRating}}
				c.JSON(http.StatusOK, result)
			} else {
				var (
					client     *mongo.Client
					err        error
					collection *mongo.Collection
				)
				clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
				client, err = mongo.Connect(context.TODO(), clientOptions.SetConnectTimeout(5*time.Second))
				if err != nil {
					fmt.Print(err)
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "connect mongo database fail",
					})
					return
				}

				// 检查连接
				err = client.Ping(context.TODO(), nil)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "connect to mongo fail",
					})
					log.Fatal(err)
				}
				log.Println("connect to mongo db!")

				collection = client.Database("test").Collection("ratings")

				findOptions := options.Find()
				findOptions.SetLimit(2)
				cursor, err := collection.Find(context.TODO(), bson.D{{}}, findOptions)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "mongo query error",
					})
					log.Fatal(err)
					return
				}
				var rate Ratings
				err = cursor.Decode(&rate)
				firstRating = rate.Rating
				cursor.Next(context.TODO())

				err = cursor.Decode(&rate)
				secondRating = rate.Rating

				result := Result{Id: productId, Ratings: Reviewer{firstRating, secondRating}}
				err = client.Disconnect(context.TODO())
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "mongo disconnect error",
					})
					log.Fatal(err)
					return
				}

				c.JSON(http.StatusOK, result)
			}
		} else {
			if os.Getenv("SERVICE_VERSION") == "v-faulty" {
				// in half of the cases return error,
				// in another half proceed as usual
				var random = rand.Float64()          // returns [0,1]
				if math.Min(random, 0.5) == random { // means random <= 0.5
					getLocalReviewsServiceUnavailable(c)
				} else {
					getLocalReviewsSuccessful(c, productId)
				}
			} else if os.Getenv("SERVICE_VERSION") == "v-delayed" {
				// in half of the cases delay for 7 seconds,
				// in another half proceed as usual
				var random = rand.Float64()          // returns [0,1]
				if math.Min(random, 0.5) == random { // means random <= 0.5
					//setTimeout(getLocalReviewsSuccessful, 7000, res, productId)
					go func(c *gin.Context, productId int) {
						ticker := time.NewTicker(time.Second * 7)
						<-ticker.C
						getLocalReviewsSuccessful(c, productId)
					}(c, productId)
				} else {
					getLocalReviewsSuccessful(c, productId)
				}
			} else if os.Getenv("SERVICE_VERSION") == "v-unavailable" || os.Getenv("SERVICE_VERSION") == "v-unhealthy" {
				if unavailable {
					getLocalReviewsServiceUnavailable(c)
				} else {
					getLocalReviewsSuccessful(c, productId)
				}
			} else {
				getLocalReviewsSuccessful(c, productId)
			}
		}

	})
	http.TimeoutHandler(r, time.Second*5, "request time out")
	r.Run(os.Args[0])
}

func putLocalReviews(productId int, ratings Reviewer) interface{} {

	userAddedRatings[productId] = Result{Id: productId, Ratings: ratings}
	return getLocalReviews(productId)
}

func getLocalReviewsServiceUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "Service unavailable",
	})
}

func getLocalReviewsSuccessful(c *gin.Context, productId int) {
	c.JSON(http.StatusOK, getLocalReviews(productId))
}

func getLocalReviews(productId int) interface{} {
	if len(userAddedRatings) < productId {
		return userAddedRatings[productId]
	}
	result := Result{Id: productId, Ratings: Reviewer{5, 4}}
	return result
}
