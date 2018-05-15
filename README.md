# golang-parallel-webcrawler
Web crawler for checking and reporting on the working status of links found on websites given to it via configuration file. Runs in parallel across multiple threads/cores with recursive goroutines calls on links found while crawling.

To start crawler, create a config.txt file with a list on urls to crawl, one on each line and add to root directory of project after cloning locally. Run:

```
git clone git@github.com:danielsmithdevelopment/golang-parallel-webcrawler.git
cd golang-parallel-webcrawler
go get -d ./...
go build -o main
./main
```