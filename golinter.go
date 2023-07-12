package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Jeffail/gabs/v2"
)

type Response struct {
	Report Report `json:"report"`
}

type Report struct {
	Errors []Error `json:"errors"`
}

type Error struct {
	RuleID      string `json:"ruleId"`
	HelpURL     string `json:"helpURL"`
	Description string `json:"description"`
	LineContent string `json:"lineContent"`
	LineNumber  int    `json:"lineNumber"`
	LinterType  string `json:"linterType"`
	Column      int    `json:"column"`
	EndColumn   int    `json:"endColumn"`
}

type Request struct {
	Source   string `json:"source"`
	Filename string `json:"filename"`
}

type Part struct {
	Source   string
	Filename string
}

const (
	configPart = iota
	codePart
	requestPart
	responsePart
	partCount // Should always be last. Used to tell how many parts there are.
)

const numberParts = partCount

const globalComponents = "globalComponents"

func main() {
	rand.Seed(time.Now().UnixNano()) // Not necessary

	flag.Parse()

	sourceFilename := flag.Arg(0)

	parts := readParts(sourceFilename)

	client := &http.Client{}

	body := gabs.New()

	body.SetP(parts[codePart].Source, "source")
	body.SetP(parts[codePart].Filename, "filename")

	parsedGlobalComponents, err := gabs.ParseJSON([]byte(parts[configPart].Source))

	if err != nil {
		log.Fatal(err)
	}

	if parsedGlobalComponents.Exists(globalComponents) {
		body.SetP(parsedGlobalComponents, "config")
	}

	formattedRequest := string(body.EncodeJSON(gabs.EncodeOptIndent("", "  ")))
	parts[requestPart].Filename = "request.json"
	parts[requestPart].Source = formattedRequest + "\n"

	fmt.Printf("Request: %s\n", string(body.EncodeJSON(gabs.EncodeOptIndent("", "  "))))

	buffer := new(bytes.Buffer)
	buffer.WriteString(string(body.EncodeJSON()))

	req, err := http.NewRequest("POST",
		"http://localhost:3000/lint-source", buffer)

	if err != nil {
		fmt.Print(err.Error())
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", os.Getenv("AXE_LINTER_API_KEY"))

	resp, err := client.Do(req)

	if err != nil {
		log.Fatal(err.Error())
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)

	if err != nil {
		fmt.Print(err.Error())
	}

	var response Response
	json.Unmarshal(bodyBytes, &response)

	b, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		fmt.Println("error:", err)
	}

	gabsResponse, err := gabs.ParseJSON(b)

	if err != nil {
		fmt.Println("error:", err)
	}

	formattedResponse := string(gabsResponse.EncodeJSON(gabs.EncodeOptIndent("", "  ")))
	fmt.Printf("Response:\n\n %s\n", formattedResponse)

	parts[responsePart].Filename = "response.json"
	parts[responsePart].Source = formattedResponse
	writeParts(sourceFilename, parts)
}

func readParts(filename string) [numberParts]Part {

	readFile, err := os.Open(filename)

	if err != nil {
		log.Fatalln(err)
	}

	defer readFile.Close()

	fileScanner := bufio.NewScanner(readFile)

	fileScanner.Split(bufio.ScanLines)

	currentFileContents := ""
	partFilename := ""

	var parts [numberParts]Part
	currentPart := configPart

	for fileScanner.Scan() {

		currentLine := fileScanner.Text()

		if strings.HasPrefix(currentLine, "---") {
			if currentFileContents != "" {
				parts[currentPart].Source = currentFileContents
				parts[currentPart].Filename = partFilename
				currentPart++
			}

			partFilename = strings.TrimPrefix(currentLine, "--- ")

			if partFilename == "" {
				log.Fatalln("no filename specified in file")
			}
			currentFileContents = ""
		} else {
			currentFileContents += currentLine + "\n"
		}

	}

	// Need to store last part because EOF indicates end of last part

	parts[currentPart].Source = currentFileContents
	parts[currentPart].Filename = partFilename

	return parts

}

func writeParts(filename string, parts [numberParts]Part) {
	// Create temporary file
	tempFilename := filename + randomString(5)

	file, err := os.Create(tempFilename)

	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	for _, part := range parts {
		file.WriteString("--- " + part.Filename + "\n")
		file.WriteString(part.Source)
	}

	file.Close()

	err = os.Remove(filename)

	if err != nil {
		log.Fatal(err)
	}

	err = os.Rename(tempFilename, filename)

	if err != nil {
		log.Fatal(err)
	}

}

func randomString(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:length]
}
