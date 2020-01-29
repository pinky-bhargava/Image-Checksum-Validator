package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// GetServiceIAMToken - Generate the Service IAM token
func GetServiceIAMToken(apiKey, ibmcloud_endpoint string) (string, error) {
	iamurl := "https://iam." + ibmcloud_endpoint + "/identity/token"
	data := url.Values{}
	data.Set("grant_type", "urn:ibm:params:oauth:grant-type:apikey")
	data.Set("response_type", "cloud_iam")
	data.Set("apikey", apiKey)

	client := &http.Client{}
	r, _ := http.NewRequest("POST", iamurl, strings.NewReader(data.Encode())) // URL-encoded payload
	r.Header.Add("Accept", "application/json")
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	responseString, err := client.Do(r)

	if err != nil {
		fmt.Println("Cannot connect ", err.Error())
		return "", err
	}
	fmt.Println(responseString.Status)

	type tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	result := tokenResponse{}

	json.NewDecoder(responseString.Body).Decode(&result)
	//fmt.Println(result)

	return "Bearer " + result.AccessToken, nil
}

func FilenameWithoutExtension(fn string) string {
	return strings.TrimSuffix(fn, path.Ext(fn))
}

// Main is the function implementing the action
func Main(params map[string]interface{}) map[string]interface{} {
	var err error
	// parse the input JSON
	endpoint, ok := params["endpoint"].(string)
	fmt.Println("endpoint=", endpoint)

	if !ok || endpoint == "" {
		fmt.Println("endpoint should not be empty")
		os.Exit(3)
	}

	// parse the input JSON
	bucket, ok := params["bucket"].(string)
	fmt.Println("bucket=", bucket)

	if !ok || bucket == "" {
		fmt.Println("bucket should not be empty")
		os.Exit(3)
	}

	// parse the input JSON
	imageName, ok := params["key"].(string)
	fmt.Println("imageName=", imageName)

	if !ok || imageName == "" {
		fmt.Println("imageName should not be empty")
		os.Exit(3)
	}
	//https://cloud-object-storage-e3-cos-standard-coz.s3.us-south.cloud-object-storage.appdomain.cloud/BIGIP-15.0.1-0.0.11.qcow2
	href := "https://" + bucket + "." + endpoint + "/"

	serviceAPIKey, ok1 := params["serviceAPIKey"].(string)

	if !ok1 {
		fmt.Println("serviceAPIKey should not be empty")
		os.Exit(3)

	}
	if serviceAPIKey == "" {
		fmt.Println("serviceAPIKey should not be empty")
		os.Exit(3)

	}

	ibmcloud_endpoint, ok1 := params["ibmcloud_endpoint"].(string)

	if ibmcloud_endpoint == "" {
		ibmcloud_endpoint = "cloud.ibm.com"
	}
	fmt.Println("ibmcloud_endpoint = ", ibmcloud_endpoint)

	msg := make(map[string]interface{})

	fmt.Println("Welcome to Image Checksum Validation service")
	//href="https://cloud-object-storage-e3-cos-standard-coz.s3.us-south.cloud-object-storage.appdomain.cloud/BIGIP-15.0.1-0.0.11.qcow2"
	iamToken, err2 := GetServiceIAMToken(serviceAPIKey, ibmcloud_endpoint)
	if err2 != nil {
		fmt.Println("Failed to generate token")
	}

	//check if both the image and its md5 exists in the cos

	extension := filepath.Ext(imageName)

	fmt.Println("Extension 1:", extension)
	imgFile := FilenameWithoutExtension(imageName)
	var cosImageURL, md5URL, md5Checksum, cosImagChecksum string
	if extension == ".md5" {
		cosImageURL = href + imgFile
		md5URL = href + imageName

	} else if extension == ".qcow2" {
		md5URL = href + imageName + ".md5"
		cosImageURL = href + imageName

	} else {
		fmt.Println("Not an image file and hence exitting...")
		os.Exit(3)
	}
	fmt.Println("md5URL: ", md5URL)
	fmt.Println("cosImageURL : ", cosImageURL)
	var isChecksumMach bool
	//get cos image checksum
	cosImagChecksum, err = GetCosEtag(cosImageURL, iamToken)

	if err != nil {
		fmt.Println("Error occured while fetching cos image checksum: ", err)
	} else {
		if cosImagChecksum != "" {
			md5Checksum, err = GetMd5FileChecksum(md5URL, iamToken)
			//fmt.Println("md5Checksum: ", md5Checksum)

			if err != nil || md5Checksum == "" {
				fmt.Println("Error occured while fetching md5 checksum: ", err)
				fmt.Println("Unable to get checksum for the file  : ", imageName)
				os.Exit(3)
			} else {
				isChecksumMach = IsChecksumMatch(md5Checksum, cosImagChecksum)

			}
		} else {
			fmt.Println("Unable to get checksum for the file  : ", imageName)

		}
	}
	msg["isChecksumMach"] = isChecksumMach

	return msg
}

// IsChecksumMatch -- perform checksum validation
func IsChecksumMatch(md5Checksum,
	cosImageChecksum string) (isChecksumMatch bool) {
	fmt.Println("In IsChecksumMatch")
	isChecksumMatch = false
	if cosImageChecksum == md5Checksum {
		isChecksumMatch = true
	} else {
		fmt.Println("checksum validation failed ")
	}
	return isChecksumMatch
}

// GetCosEtag - Get COS file md5 checksum by reading head of the file
func GetCosEtag(href, iamToken string) (string, error) {
	var (
		err  error
		req  *http.Request
		resp *http.Response
		eTag string
	)
	fmt.Println("In GetCosEtag")

	req, err = http.NewRequest(http.MethodHead, href, nil)
	if err != nil {
		fmt.Println(" GotCosEtag(): HEAD request failed! ")
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", iamToken)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(" GotCosEtag(): Do request failed! ", err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		fmt.Println("cos file not found..")
		return "", errors.New("qcow2 file not found")

	}
	eTag = resp.Header.Get("Etag")
	if eTag == "" {
		fmt.Println(" gotCosEtag(): Failed to get ret Etag from response! ")
		return "", err
	}
	//	fmt.Println("eTag ", eTag)
	fmt.Println("Exit getCosEtag()")

	// eTag output is with double quoted. So, ignoring first and last byte
	return eTag[1 : len(eTag)-1], nil
}

// GetMd5FileChecksum - Read .md5 file and return it in string format
func GetMd5FileChecksum(
	href, iamToken string) (string, error) {
	resp, err := postRequesttoCOS(href, http.MethodGet, iamToken)

	var (
		md5 []byte
	)

	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		fmt.Println("md5 file not found")
		return "", errors.New("md5 file not found")

	}
	// Response looks like below example
	// Ex.:
	// 47e1129de33c8e010496ac1b70833401 UT.qcow2.zip
	md5, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(" gotCosMd5FromFile(): Failed to read the response! ", err)
		return "", err
	}

	md5Checksum := strings.Split(string(md5), " ")

	fmt.Println("Exit getCosMd5FromFile() ")

	return md5Checksum[0], nil
}

func postRequesttoCOS(href, method, iamToken string) (*http.Response, error) {

	var (
		req  *http.Request
		resp *http.Response
		err  error
	)

	fmt.Println("In postRequesttoCOS()")

	req, err = http.NewRequest(method, href, nil)
	if err != nil {
		fmt.Println(" Error found : ", err)
		return resp, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", iamToken)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error : ", err)
		return resp, err
	}
	return resp, err
}
