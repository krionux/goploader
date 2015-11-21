package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nu7hatch/gouuid"

	"github.com/Depado/goploader/server/conf"
	"github.com/Depado/goploader/server/utils"
)

var db gorm.DB
var timeLimit = 30 * time.Minute

// ResourceEntry represents the data stored in the database
type ResourceEntry struct {
	gorm.Model
	Key string
}

func create(w http.ResponseWriter, r *http.Request) {
	var err error
	if r.Method == "GET" {
		return
	}
	log.Printf("[INFO][%s]\tReceiving data.", r.Host)
	u, err := uuid.NewV4()
	if err != nil {
		log.Printf("[ERROR][%s] During creation of uuid : %s\n", r.Host, err)
		http.Error(w, http.StatusText(503), 503)
		return
	}
	br := bufio.NewReaderSize(r.Body, 512)
	path := conf.C.UploadDir + u.String()
	file, err := os.Create(path)
	if err != nil {
		log.Printf("[ERROR][%s]\tDuring file creation : %s\n", r.Host, err)
		http.Error(w, http.StatusText(503), 503)
		return
	}
	defer file.Close()
	_, err = io.Copy(file, br)
	if err != nil {
		log.Printf("[ERROR][%s]\tDuring writing file : %s\n", r.Host, err)
		http.Error(w, http.StatusText(503), 503)
		return
	}
	e := ResourceEntry{}
	e.Key = u.String()
	db.Create(&e)
	fmt.Fprint(w, "http://"+conf.C.NameServer+"/view/"+u.String()+"\n")
	return
}

func view(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/view/"):]
	re := ResourceEntry{}
	db.Where(&ResourceEntry{Key: id}).First(&re)
	if re.Key == "" {
		log.Printf("[INFO][%s]\tNot found : %s", r.Host, id)
		http.Error(w, http.StatusText(404), 404)
		return
	}
	http.ServeFile(w, r, conf.C.UploadDir+re.Key)
}

func monit() {
	tc := time.NewTicker(1 * time.Minute)
	for {
		res := []ResourceEntry{}
		db.Find(&res, "created_at < ?", time.Now().Add(-timeLimit))
		db.Unscoped().Where("created_at < ?", time.Now().Add(-timeLimit)).Delete(&ResourceEntry{})
		if len(res) > 0 {
			log.Printf("[INFO][System]\tFlushing %d DB entries and files.\n", len(res))
		}
		for _, re := range res {
			err := os.Remove(conf.C.UploadDir + re.Key)
			if err != nil {
				log.Printf("[ERROR][System]\tWhil deleting : %v", err)
			}
		}
		<-tc.C
	}
}

func main() {
	var err error

	confPath := flag.String("c", "conf.yml", "Local path to configuration file.")
	flag.Parse()

	if err = conf.Load(*confPath); err != nil {
		log.Fatal(err)
	}

	if err = utils.EnsureDir(conf.C.UploadDir); err != nil {
		log.Fatal(err)
	}

	db, err = gorm.Open("sqlite3", conf.C.DB)
	if err != nil {
		log.Fatal(err)
	}
	db.AutoMigrate(&ResourceEntry{})
	log.Printf("[INFO][System]\tStarted goploader server on port %d.\n", conf.C.Port)
	go monit()
	log.Println("[INFO][System]\tStarted monitoring of files and db entries.")

	http.HandleFunc("/", create)
	http.HandleFunc("/view/", view)
	http.ListenAndServe(fmt.Sprintf(":%d", conf.C.Port), nil)
}