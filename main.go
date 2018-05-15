package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"bufio"
	"os"
	"runtime"

	"golang.org/x/net/html"

	"gopkg.in/gomail.v2"
)

// Node structure represents links found
type Node struct {
	link           string
	parent         string
	linkType       string
	externalDomain bool
}

// root url to start with passed in from txt file
var rootURL string

// link nodes collected here
var alreadyCrawled map[string]bool
var brokenLinks []Node
var linkToLinks map[Node][]Node

// Semaphore to limit number of get requests
var semaphore = make(chan int, runtime.GOMAXPROCS(0))

// mutex allows maps to safely be edited by different goroutines
var mutex sync.Mutex
var waitGroup sync.WaitGroup

func main() {
	// read config.txt and store links in slice
	linksToCrawl, err := readLines("config.txt")

	// check for errors while reading config file
	if err != nil {
		fmt.Println("Error reading file: ", err)
	}

	// create slice of strings to output
	var results []string

	// crawl each website in input file one consecutively
	for i := 0; i < len(linksToCrawl); i++ {
		var emailContent []string
		// set root url to start with
		rootURL = linksToCrawl[i]
		fmt.Println(rootURL)

		// declare initial node
		root := Node{rootURL, "root", "Link", false}

		// create title for site results and append title to output
		var resultsTitle = "<h3>Report for: " + root.link + "</h3><br>"
		results = append(results, resultsTitle)
		emailContent = append(emailContent, resultsTitle)
		// instantiate maps to collect links
		brokenLinks = make([]Node, 0)
		linkToLinks = make(map[Node][]Node)
		alreadyCrawled = make(map[string]bool)

		// initiate crawling process
		crawl(root)

		sum := 0

		for _, eachLink := range linkToLinks {

			sum += len(eachLink)
		}
		fmt.Println("Detected Number of links found ", sum)
		noOfCrawledLinks := 0
		for eachLink := range alreadyCrawled {
			// results = append(results, eachLink.link)
			fmt.Println(eachLink)
			noOfCrawledLinks++
		}
		fmt.Printf("\n")
		fmt.Println("Detected Number of links found ", sum)
		fmt.Printf("Number of links crawled: %d \n\n", noOfCrawledLinks)

		if len(brokenLinks) == 0 {
			fmt.Println("No broken links found.")
			results = append(results, "No broken links found.")
			emailContent = append(emailContent, "<h3>No broken links found.</h3>")
		} else {
			for i := 0; i < len(brokenLinks); i++ {
				if brokenLinks[i].link != "" {
					fmt.Printf("Broken link : %s \n", brokenLinks[i].link)
					emailContent = append(emailContent, "Link Type: "+brokenLinks[i].linkType+"<br>Link URL: <a href='"+brokenLinks[i].link+"'>"+brokenLinks[i].link+"</a>"+"<br>Source: "+brokenLinks[i].parent+"<br><br>")
					results = append(results, "Link Type: "+brokenLinks[i].linkType+"<br>Link URL: <a href='"+brokenLinks[i].link+"'>"+brokenLinks[i].link+"</a>"+"<br>Source: <a href='"+brokenLinks[i].parent+"'>"+brokenLinks[i].parent+"</a><br><br>")
				}
			}
			fmt.Printf("\n")
			fmt.Println(len(brokenLinks), "broken links found")
		}

		results = append(results, "\n")
		writeLines(results, "output.html")
		result := strings.Join(emailContent, "\n")
		sendEmail(result) //send mail to webmaster@loni.usc.edu
	}

	writeLines(results, "output.html") //making log file
	// for key,value:= range linkToLinks{
	// 	fmt.Println(key,value)
	// 	fmt.Println("\n\n")
	// }
	waitGroup.Wait()
}

func readLines(inputPath string) ([]string, error) {
	// open file located at input path
	inputFile, err := os.Open(inputPath)

	// check for errors while opening file
	if err != nil {
		return nil, err
	}
	defer inputFile.Close()

	// slice to store file contents line by line
	var fileContents []string
	fileScanner := bufio.NewScanner(inputFile)
	for fileScanner.Scan() {
		// read line into slice
		fileContents = append(fileContents, fileScanner.Text())
	}
	return fileContents, fileScanner.Err()
}

// writeLines writes the lines to the given file.
func writeLines(outputContent []string, outputPath string) error {
	// create file at output path
	outputFile, err := os.Create(outputPath)

	// check for errors while creating file
	if err != nil {
		return err
	}
	defer outputFile.Close()

	fileWriter := bufio.NewWriter(outputFile)
	for _, item := range outputContent {
		// write item to file
		fmt.Fprintln(fileWriter, item)
	}
	return fileWriter.Flush()
}

func sendEmail(emailContent string) {
	// create new email and set headers
	email := gomail.NewMessage()
	email.SetHeader("From", "fromaddress@email.com")
	email.SetHeader("To", "toaddress@email.com")
	email.SetHeader("Subject", "["+rootURL+"] Broken Links Detected")
	// set email body to passed in email content
	email.SetBody("text/html", emailContent)
	// create dialer with credentials
	// dialer := gomail.NewDialer("smtp.gmail.com", 25, "fromaddress@email.com", "A2S2werg5Frts")

	// // Send the email
	// if err := dialer.DialAndSend(email); err != nil {
	// 	log.Panic(err)
	// }
}

func crawl(root Node) {
	// check semaphore to make sure a core is available to use
	semaphore <- 1
	// create channel of nodes for queue
	urlQueue := make(chan []Node, 1000)
	// perform GET request on link passed in
	resp, err := http.Get(string(root.link))
	// check for errors from http request
	if err != nil {
		fmt.Println("error is ", err)
	}
	// start a goroutine to extract links from http response into url queue
	go getLinks(resp, root, urlQueue)
	defer resp.Body.Close()
	// initialize job counter and set root link to already crawled
	jobCounter := 1
	alreadyCrawled[root.link] = true
	for jobCounter > 0 {
		jobCounter--
		fmt.Println("len of chan ", len(urlQueue))
		// pull next job from queue and store it
		next := <-urlQueue
		// check all links in job
		for _, url := range next {
			// make sure link hasn't been crawled before
			if _, done := alreadyCrawled[trimHash(url.link)]; !done {
				// make sure link is not an empty string
				if url.link != "" {
					timeout := time.Duration(10 * time.Second)
					client := http.Client{Timeout: timeout}
					// perform http GET request for current link in job
					resp, err := client.Get(trimHash(strings.TrimSpace(url.link)))
					// check for errors and log them
					if err != nil {
						log.Println("Error while crawling ", url, "is: ", err)
						alreadyCrawled[trimHash(url.link)] = true
					}
					if resp != nil {
						// if link is broken, add it to list of broken links
						if resp.StatusCode == 404 {
							brokenLinks = append(brokenLinks, url)
						} else {
							// check to see if link is an internal link
							if !url.externalDomain && url.linkType == "Link" {
								fmt.Println("started crawling: ", url.link, url.externalDomain, alreadyCrawled[url.link])
								fmt.Println("from parent url: ", url.parent)
								fmt.Println()
								// increment job counter and extract links found on current links
								jobCounter++
								go getLinks(resp, url, urlQueue)
								alreadyCrawled[trimHash(url.link)] = true
							}
						}
					}
				}
			}
		}
	}
	// release hold to semaphore once job completed
	<-semaphore
	return
}

func getLinks(resp *http.Response, parent Node, Urls chan []Node) {
	// increment waitgroup by 1 job
	waitGroup.Add(1)
	var links = make([]Node, 0)
	// create tokenizer to parse html from  http response
	htmlTokenizer := html.NewTokenizer(resp.Body)
	for {
		// grab next token and check its type
		htmlTokenizer.NextIsNotRawText()
		tokenType := htmlTokenizer.Next()
		switch {
		case tokenType == html.ErrorToken:
			// lock map before storing links found
			mutex.Lock()
			linkToLinks[parent] = links
			mutex.Unlock()

			// add found links to queue of links to crawl
			Urls <- links
			//fmt.Println(len(Urls))
			// remove 1 job from wait group
			waitGroup.Done()
			return

		case tokenType == html.StartTagToken:
			token := htmlTokenizer.Token()
			isAnchor := token.Data == "a"
			linkType := ""
			// check it token is an anchor tag
			if isAnchor {
				linkType = "Link"
				// iterate through attributes to find href
				for _, attribute := range token.Attr {
					base, err := url.Parse(rootURL)
					if err != nil {
						log.Println("err is ", err)
					}
					if attribute.Key == "href" {
						urlString := trimHash(strings.TrimSpace(attribute.Val))
						if urlString != parent.link {
							// create new node for link found
							var newNode Node
							if strings.Contains(urlString, "mailto:") {
								linkType = "Email address"
								if !isEmailAddress(urlString[7:]) {
									brokenNode := Node{urlString, parent.link, linkType, true}
									brokenLinks = append(brokenLinks, brokenNode)
								}
								newNode = Node{urlString, parent.link, linkType, true}
							} else if strings.Contains(urlString, "tel:") {
								continue
							} else {
								// parsing the url
								u, err := url.Parse(urlString)
								if err != nil {
									log.Println(err)
								} else {
									uri := base.ResolveReference(u)
									urlString = uri.String()
									//checking outsideLink
									if strings.Contains(urlString, rootURL) {
										newNode = Node{urlString, parent.link, linkType, false}
									} else {
										newNode = Node{urlString, parent.link, linkType, true}
									}
								}

							}

							if !strings.Contains(newNode.link, "mailto:") && !alreadyCrawled[newNode.link] {
								links = append(links, newNode)
							}
							break
						}
					}
				}
			}
		}
	}
}

func isEmailAddress(emailAddress string) bool {
	// see if passed in string contains the '@' symbol
	if strings.ContainsAny(emailAddress, "@") == true {
		// locate index of '@' symbol and parse string
		indexOfAt := strings.Index(emailAddress, "@")
		domain := emailAddress[indexOfAt+1:]
		// see if parsed string contains a top level domain
		if strings.ContainsAny(domain, ".") == true {
			indexOfDot := strings.Index(domain, ".")
			topLevelDomain := domain[indexOfDot+1:]
			// return true if the top level domain length > 0
			if len(topLevelDomain) > 0 {
				return true
			}
		}
	}
	return false
}

// trimHash slices a hash # from the link
func trimHash(link string) string {
	if strings.ContainsAny(link, "#") {
		index := strings.Index(link, "#")
		return link[:index]
	}
	return link
}
