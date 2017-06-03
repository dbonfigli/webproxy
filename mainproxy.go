package main

import (
	"encoding/base64"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

func marshalURL(urlp *url.URL) string {
	return base64.URLEncoding.EncodeToString([]byte(urlp.String()))
}

func unmarshalURL(s string) *url.URL {
	byteURL, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		log.Fatal(err)
	}
	urlp, err := url.Parse(string(byteURL))
	if err != nil {
		log.Fatal(err)
	}
	return urlp
}

//fai diventare una url relativa in assoluta
func sanitizeURL(urlToSanitize *url.URL, currentURLp *url.URL) url.URL {

	sanitizedURL := *urlToSanitize

	// se non c'era host, imposto quello corrente
	if sanitizedURL.Host == "" {
		sanitizedURL.Host = currentURLp.Host
	}

	// se non c'era scheme, imposto quella corrente
	if sanitizedURL.Scheme == "" {
		sanitizedURL.Scheme = currentURLp.Scheme
	}

	// pulisci la path, assumi che la url corrente sia SEMPRE assoluta
	if sanitizedURL.Path != "" && !path.IsAbs(sanitizedURL.Path) {
		sanitizedURL.Path = path.Clean(path.Join("/", path.Dir(currentURLp.Path), sanitizedURL.Path))
	}

	return sanitizedURL
}

func replaceURLhtml(urlString string, element string, currentURLp *url.URL) string {

	//log.Println("url da riscrivere: " + urlString)
	//prendo la vera url, go non ha lookahead, stronzi
	//devo inoltre preservare i quote

	newurlString := ""
	quote := ""
	if strings.Contains(urlString, `"`) {
		newurlString = strings.Replace(urlString, `"`, ``, -1)
		quote = `"`
	}
	if strings.Contains(newurlString, `'`) {
		newurlString = strings.Replace(newurlString, `'`, ``, -1)
		quote = `'`
	}

	newurlString = strings.Replace(newurlString, element, ``, -1)

	//log.Println("prima: " + newurlString)

	//ora la parso e inserisco delle info di controno, poi la serializzo e ritorno questa nuova url

	urlp, err := url.Parse(newurlString)
	if err != nil {
		//log.Println("ignoro url che non ho potuto parsare:")
		//log.Println(err)
		return urlString
	}

	//i fragment non devono essere inclusi nel marshal, tanto non sono passati via http
	fragment := ""
	if urlp.Fragment != "" {
		fragment = "#" + urlp.Fragment
		urlp.Fragment = ""
	}

	// certe volte arrivano vuote, in quel caso metti solo il fragment
	if urlp.Path == "" && urlp.Host == "" {
		return element + quote + fragment + quote
	}

	//ora ho una url ma potrebbe avere path relativo, devo farlo assoluto
	newURL := sanitizeURL(urlp, currentURLp)

	//log.Println("dopo:  " + newURL.String() + fragment)

	returnURLstring := element + quote + `/go?u=` + marshalURL(&newURL) + fragment + quote
	return returnURLstring

}

func replaceURLcss(urlString string, element string, currentURLp *url.URL) string {

	//pulisco contorni
	newurlString := strings.Replace(urlString, `url(`, ``, -1)
	newurlString = strings.Replace(newurlString, `)`, ``, -1)
	newurlString = strings.Replace(newurlString, `'`, ``, -1)
	newurlString = strings.Replace(newurlString, `"`, ``, -1)

	//la parso
	urlp, err := url.Parse(newurlString)
	if err != nil {
		log.Println("ignoro url che non ho potuto parsare:")
		log.Println(err)
		return urlString
	}

	//la faccio diventare assoluta
	newURL := sanitizeURL(urlp, currentURLp)

	//elimino il fragment prima del marshall e lo aggiungo alla fine
	fragment := ""
	if newURL.Fragment != "" {
		fragment = "#" + newURL.Fragment
		newURL.Fragment = ""
	}

	returnURLstring := `url('/go?u=` + marshalURL(&newURL) + fragment + `')`
	return returnURLstring
}

func transformPage(b string, originalURL *url.URL) string {

	//occupiamoci ora di tutti gli href
	replaceURLelem := func(urlString string) string {
		return replaceURLhtml(urlString, "href=", originalURL)
	}
	re := regexp.MustCompile(`href=("|')(.*?)("|')`)
	transformed := re.ReplaceAllStringFunc(b, replaceURLelem)

	//occupiamoci ora di tutti gli src
	replaceURLelem = func(urlString string) string {
		return replaceURLhtml(urlString, "src=", originalURL)
	}
	re = regexp.MustCompile(`src=("|')(.*?)("|')`)
	transformed = re.ReplaceAllStringFunc(transformed, replaceURLelem)

	//occupiamoci delle url css
	replaceURLelem = func(urlString string) string {
		return replaceURLcss(urlString, "url(", originalURL)
	}
	re = regexp.MustCompile(`url\((.*?)\)`)
	transformed = re.ReplaceAllStringFunc(transformed, replaceURLelem)

	return transformed

}

func gopage(w http.ResponseWriter, r *http.Request) {

	//log.Println("fetch url gopage: " + r.URL.String())
	//fmt.Println("GET params were:", r.URL.Query())

	// mi accerto di avere i dati sulla pagina da raccogliere
	urlParam, urlexists := r.URL.Query()["u"]
	if urlexists == false || len(urlParam) != 1 || urlParam[0] == "" {
		io.WriteString(w, "errore url")
		return
	}
	pageurl := unmarshalURL(urlParam[0])

	// vado a pescare la pagina richiesta
	response, err := http.Get(pageurl.String())
	if err != nil {
		io.WriteString(w, "errore on getting page encoded in param u")
		return
	}
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		io.WriteString(w, "errore read body")
		return
	}

	mimetype := response.Header.Get("Content-Type")
	//log.Println(pageurl.String() + " " + mimetype)

	//ora riscrivo il contenuto se e' css o html
	if strings.Contains(mimetype, `text/css`) || strings.Contains(mimetype, `text/html`) {
		respString := transformPage(string(body), pageurl)
		w.Header().Set("Content-Type", mimetype)
		io.WriteString(w, respString)
	} else {
		io.WriteString(w, string(body))
	}

}

func mainpage(w http.ResponseWriter, r *http.Request) {

	respString := `
	<!DOCTYPE html> 
	<html>
	<body>
	<form action="">
	tell us where to go:
	<input type="text" name="u" value="">
	<input type="submit" value="Submit">
	</form>
	</br>
 	`

	log.Println("no go page, must fetch url " + r.URL.String())
	//fmt.Println("GET params were:", r.URL.Query())
	urlParam, urlexists := r.URL.Query()["u"]
	if urlexists && len(urlParam) == 1 && urlParam[0] != "" {
		urlp, err := url.Parse(string(urlParam[0]))
		if err != nil {
			log.Fatal(err)
		}
		urlp.Scheme = "http"
		marshalledURL := marshalURL(urlp)
		respString += `<iframe style="width: 100%; height: 550px " src="/go?u=` +
			marshalledURL + `"><p>Your browser does not support iframes</p></iframe></body></html>`
	} else {
		respString += "</body></html>"
	}
	io.WriteString(w, respString)
}

func main() {
	http.HandleFunc("/go", gopage)
	http.HandleFunc("/", mainpage)
	http.ListenAndServe(":8000", nil)
	// devo parsare solo quelli con mime type text/html, forse anche text/css
}
