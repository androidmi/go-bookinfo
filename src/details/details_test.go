package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"testing"
)

func TestJsonConvert(t *testing.T) {
	data, err := ioutil.ReadFile("book.json")
	var volumes BookVolumes
	if err = json.Unmarshal(data, &volumes); err != nil {
		log.Fatal(err)
	}
	//fmt.Println(string(data))
	fmt.Printf("items size : %v \n", len(volumes.Items))

	book := volumes.Items[0]
	language := book.VolumeInfo.Language
	fmt.Println(language)
}

func TestRequestInfo(t *testing.T) {
	resp, err := http.Get(fmt.Sprintf("https://www.googleapis.com/books/v1/volumes?q=isbn:%s", "0486424618"))
	if err != nil {
		log.Fatal(err, "get")
	}

	if os.Getenv("DO_NOT_ENCRYPT") == "true" {
		// enable ssl
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(string(body))

}
