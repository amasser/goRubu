package services

//contains function to create a shortened url
import (
	"context"
	"encoding/base64"
	cacheConnection "goRubu/cache"
	dao "goRubu/daos"
	model "goRubu/models"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/joho/godotenv"
)

var mc *memcache.Client

//var CACHE_EXPIRATION int64
var err error

// EXPIRY_TIME - TTL for an item int cache
var EXPIRY_TIME int

func init() {
	dir, _ := os.Getwd()
	envFile := "variables.env"
	if strings.Contains(dir, "test") {
		envFile = "../variables.env"
	}

	if err := godotenv.Load(envFile); err != nil {
		log.Fatal("Unable to load env file from urlCreationService Init", err)
	}

	// memcached connection
	mc = cacheConnection.CreateCon()
}

// CreateShortenedUrl - This service shortens a give url.
func CreateShortenedUrl(inputUrl string) string {

	counterVal := dao.GetCounterValue()
	newUrl := GenerateShortenedUrl(counterVal)
	inputModel := model.UrlModel{UniqueId: counterVal, Url: inputUrl, CreatedAt: time.Now()}

	//first update the cache with (key,val) => (newUrl, inputUrl)
	err = mc.Set(&memcache.Item{
		Key:        newUrl,
		Value:      []byte(inputUrl),
		Expiration: int32(EXPIRY_TIME),
	})

	if err != nil {
		log.Printf("Error in setting memcached value:%v", err)
	}

	// FIXME:
	// Race Condition - Undesirable condition where o/p of a program depends on the seq of execution of go routines

	// To prevent this use Mutex - a locking mechanism, to ensure only one Go routine
	// is running in the CS at any point of time

	// TODO handle Race Conditions. Also use transaction to enable consistency.
	// You could have mutexes as well, but mutex would have guaranteed consistency.
	dao.InsertInShortenedUrl(inputModel)
	dao.UpdateCounter()
	return newUrl
}

//UrlRedirection - will return back the original url from which the inputUrl was created
func UrlRedirection(inputUrl string) string {
	// try hitting the cache first
	// stored as "https://goRubu/MTW" -> "www.google.com"

	url, err := mc.Get(inputUrl)
	if err == nil {
		log.Println("Shortened url found in cache", string(url.Value))
		return string(url.Value)
	} else if err != memcache.ErrCacheMiss {
		log.Fatal("Memcached error ", err)
	}

	// if its a cache miss, fetch the value from db and update the cache.
	// https://goRubu/MTAwMDE=
	i := strings.Index(inputUrl, "Rubu/")
	encodedForm := inputUrl[i+5:]

	byteNumber, _ := base64.StdEncoding.DecodeString(encodedForm)
	UniqueId, _ := strconv.Atoi(string(byteNumber))
	urlModel := dao.GetUrl(UniqueId)

	err2 := mc.Set(&memcache.Item{
		Key:        inputUrl,
		Value:      []byte(urlModel.Url),
		Expiration: int32(EXPIRY_TIME),
	})

	if err2 != nil {
		log.Fatal("Error in writing Memcached Value ", err2)
	}

	// urlMode.Url will be "", if the given shortened url does't exists in db.
	return urlModel.Url
}

// RemovedExpiredEntries -removed the db entries that are in the db for more than three min. this function is being run by a cron after every 5 min
func RemovedExpiredEntries() {

	cur := dao.GetAll()

	for cur.Next(context.TODO()) {
		var input model.UrlModel
		if err := cur.Decode(&input); err != nil {
			log.Fatal("Error while decoding cursor value into model ", err)
		}

		// unique id is the counter val
		// we need to delete url from cache as well
		// but as our expiry time for db and cache is same. We dont need to delete it manually
		// (key->val) = (https://fs.com -> www.google.com)

		var start time.Time = input.CreatedAt
		a := time.Since(start)
		b := a.Seconds()

		if b > float64(EXPIRY_TIME) {
			dao.CleanDb(input.UniqueId)
		}
	}

}

// GenerateShortenedUrl - It will take a int, and encode it in base64
func GenerateShortenedUrl(counterVal int) string {
	byteNumber := []byte(strconv.Itoa(counterVal))
	tempUrl := base64.StdEncoding.EncodeToString(byteNumber)

	newUrl := "https://goRubu/" + tempUrl
	return newUrl
}
