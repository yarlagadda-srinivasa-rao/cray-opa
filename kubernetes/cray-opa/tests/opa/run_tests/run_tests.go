// MIT License
//
// (C) Copyright 2021-2022 Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/golang-jwt/jwt"
)

type tokenCreator struct {
	key []byte
}

type createTokenArgs struct {
	role   string
	issuer string
	aud    string
	sub    string
	typ    string
}

func (t tokenCreator) create(args createTokenArgs) (string, error) {
	var err error
	atClaims := jwt.MapClaims{}
	atClaims["iss"] = args.issuer
	atClaims["exp"] = time.Now().Add(time.Minute * 15).Unix()
	atClaims["aud"] = []string{args.aud}
	if args.role != "" {
		atClaims["resource_access"] = map[string]interface{}{
			"shasta": map[string]interface{}{
				"roles": []string{args.role},
			},
		}
	}
	if args.sub != "" {
		atClaims["sub"] = args.sub
	}
	if args.typ != "" {
		atClaims["typ"] = args.typ
	}
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	token, err := at.SignedString(t.key)
	if err != nil {
		return "", err
	}
	return token, nil
}

func main() {
	var xnameEnablement bool
	flag.BoolVar(&xnameEnablement, "x", false, "Enable xname validation")
	flag.Parse()

	policyTemplateFilename := flag.Arg(0)
	testTemplateFilename := flag.Arg(1)

	randomKey := make([]byte, 32)
	rand.Read(randomKey)

	tc := tokenCreator{key: randomKey}

	randomKeyB64 := base64.RawURLEncoding.EncodeToString(randomKey)

	jwksResponseData := map[string]interface{}{
		"keys": []interface{}{
			map[string]interface{}{
				"kid": "MoeoCPOcAKjkbMy7k-IhGMtjvqehZqRTqevioCtoaNM",
				"kty": "oct",
				"k":   randomKeyB64,
				"alg": "HS256",
			},
		},
	}

	jwksResponse, err := json.Marshal(jwksResponseData)
	if err != nil {
		log.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("JWKS FETCHED!")
		fmt.Fprintln(w, string(jwksResponse))
	}))
	defer ts.Close()

	keycloakIssuer := "http://keycloak1"
	spireIssuer := "http://spire.local/shasta/vshastaio"
	shastaAud := "shasta"
	systemComputeAud := "system-compute"

	var spireSubNCNPrefix string
	var spireSubComputePrefix string
	if xnameEnablement {
		spireSubNCNPrefix = "spiffe://shasta/ncn/ncnw001/workload/"
		spireSubComputePrefix = "spiffe://shasta/compute/x1/workload/"
	} else {
		spireSubNCNPrefix = "spiffe://shasta/ncn/workload/"
		spireSubComputePrefix = "spiffe://shasta/compute/workload/"
	}
	var spireSub string

	args := createTokenArgs{
		role: "admin", issuer: keycloakIssuer, aud: shastaAud, typ: "Bearer",
	}
	adminToken, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("admin token:", adminToken)

	args = createTokenArgs{role: "user", issuer: keycloakIssuer, aud: shastaAud, typ: "Bearer"}
	userToken, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("user token:", userToken)

	args = createTokenArgs{
		role: "admin", issuer: keycloakIssuer, aud: shastaAud, typ: "Invalid",
	}
	invalidTypAdminToken, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Invalid typ admin token:", invalidTypAdminToken)

	args = createTokenArgs{
		role: "system-pxe", issuer: keycloakIssuer, aud: shastaAud, typ: "Bearer",
	}
	pxeToken, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("pxe token:", pxeToken)

	args = createTokenArgs{
		role: "system-compute", issuer: keycloakIssuer, aud: shastaAud, typ: "Bearer",
	}
	computeToken, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("compute token:", computeToken)

	args = createTokenArgs{role: "wlm", issuer: keycloakIssuer, aud: shastaAud, typ: "Bearer"}
	wlmToken, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("wlm token:", wlmToken)

	spireSub = "spiffe://shasta/invalid"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireInvalidSub, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireInvalidSub)

	spireSub = spireSubNCNPrefix + "cfs-state-reporter"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnCfsStateReporter, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnCfsStateReporter)

	spireSub = spireSubNCNPrefix + "ckdump"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnCkdump, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnCkdump)

	spireSub = spireSubNCNPrefix + "ckdump_helper"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnCkdumpHelper, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnCkdumpHelper)

	spireSub = spireSubNCNPrefix + "cpsmount"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnCpsmount, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnCpsmount)

	spireSub = spireSubNCNPrefix + "cpsmount_helper"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnCpsmountHelper, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnCpsmountHelper)

	spireSub = spireSubNCNPrefix + "dvs-hmi"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnDvsHmi, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnDvsHmi)

	spireSub = spireSubNCNPrefix + "dvs-map"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnDvsMap, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnDvsMap)

	spireSub = spireSubNCNPrefix + "heartbeat"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnHeartbeat, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnHeartbeat)

	spireSub = spireSubNCNPrefix + "orca"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnOrca, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnOrca)

	spireSub = spireSubNCNPrefix + "tpm-provisioner"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireNcnTPMProvisioner, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireNcnTPMProvisioner)

	spireSub = spireSubComputePrefix + "cfs-state-reporter"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeCfsStateReporter, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeCfsStateReporter)

	spireSub = spireSubComputePrefix + "ckdump"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeCkdump, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeCkdump)

	spireSub = spireSubComputePrefix + "ckdump_helper"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeCkdumpHelper, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeCkdumpHelper)

	spireSub = spireSubComputePrefix + "cpsmount"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeCpsmount, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeCpsmount)

	spireSub = spireSubComputePrefix + "cpsmount_helper"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeCpsmountHelper, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeCpsmountHelper)

	spireSub = spireSubComputePrefix + "dvs-hmi"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeDvsHmi, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeDvsHmi)

	spireSub = spireSubComputePrefix + "dvs-map"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeDvsMap, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeDvsMap)

	spireSub = spireSubComputePrefix + "heartbeat"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeHeartbeat, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeHeartbeat)

	spireSub = spireSubComputePrefix + "orca"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeOrca, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeOrca)

	spireSub = spireSubComputePrefix + "wlm"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeWlm, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeWlm)

	spireSub = spireSubComputePrefix + "tpm-provisioner"
	args = createTokenArgs{
		issuer: spireIssuer, aud: systemComputeAud, sub: spireSub,
	}
	spireComputeTPMProvisioner, err := tc.create(args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(spireSub, ":", spireComputeTPMProvisioner)

	// Reading in the policy template file and generating policy file.

	yf, err := ioutil.ReadFile(policyTemplateFilename)
	if err != nil {
		log.Fatal(err)
	}

	// Isolate policy from yaml
	tSplit := strings.Split(string(yf), "\n")
	var l int
	for i, v := range tSplit {
		if v == "  policy.rego: |-" {
			l = i + 1
			break
		}
	}
	tSplit = tSplit[l : len(tSplit)-2]
	tSplit = append([]string{"{{ define \"test.policy\" }}"}, tSplit...)

	// Add missing default deny from base policy
	for i, v := range tSplit {
		if v == "" {
			l = i
			break
		}
	}

	tSplit = append(tSplit[:l+1], tSplit[l:]...)
	tSplit[l] = "    default allow = { \"allowed\": false, \"headers\": {\"x-ext-auth-allow\": \"no\"}, \"body\": \"Unauthorized Request\", \"http_status\": 403 }"

	// Fix variables
	dat := []byte(strings.Join(tSplit, "\n"))
	changeOptions := regexp.MustCompile(".options.issuers")
	dat = changeOptions.ReplaceAll(dat, []byte(".Values.issuers"))

	tpl := template.Must(
		template.New("base").Funcs(sprig.FuncMap()).Parse(string(dat)))

	var values map[string]interface{}

	if xnameEnablement {
		values = map[string]interface{}{
			"Values": map[string]interface{}{
				"opa": map[string]interface{}{
					"xnamePolicy": map[string]interface{}{
						"enabled":   true,
						"bos":       true,
						"cfs":       true,
						"dvs":       true,
						"heartbeat": true,
					},
				},
				"requireHeartbeatToken": true,
				"issuers":               []string{keycloakIssuer},
				"jwtValidation": map[string]interface{}{
					"keycloak": map[string]interface{}{
						"jwksUri": ts.URL,
					},
					"spire": map[string]interface{}{
						"jwksUri":     ts.URL,
						"issuers":     []string{spireIssuer},
						"trustDomain": "shasta",
					},
				},
			},
		}
	} else {
		values = map[string]interface{}{
			"Values": map[string]interface{}{
				"issuers": []string{keycloakIssuer},
				"jwtValidation": map[string]interface{}{
					"keycloak": map[string]interface{}{
						"jwksUri": ts.URL,
					},
					"spire": map[string]interface{}{
						"jwksUri":     ts.URL,
						"issuers":     []string{spireIssuer},
						"trustDomain": "shasta",
					},
				},
			},
		}
	}

	log.Printf("Values: %v\n", values)

	f, err := os.Create("policy.rego")
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	fmt.Println("Rendering policy template to 'policy.rego'")
	err = tpl.ExecuteTemplate(f, "test.policy", values)
	if err != nil {
		log.Fatal(err)
	}

	f.Close()

	fmt.Println("Rendered policy template:")
	fmt.Println("*****")

	dat, err = ioutil.ReadFile("policy.rego")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s", string(dat))

	fmt.Println("*****")

	// Reading in the test template file and creating test file.
	dat, err = ioutil.ReadFile(testTemplateFilename)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Read the test template file %v", string(dat))

	tpl = template.Must(
		template.New("base").Funcs(sprig.FuncMap()).Parse(string(dat)))

	values = map[string]interface{}{
		"userToken":            userToken,
		"adminToken":           adminToken,
		"invalidTypAdminToken": invalidTypAdminToken,
		"pxeToken":             pxeToken,
		"computeToken":         computeToken,
		"wlmToken":             wlmToken,
		"spire": map[string]interface{}{
			"invalidSub": spireInvalidSub,
			"ncn": map[string]interface{}{
				"cfs_state_reporter": spireNcnCfsStateReporter,
				"ckdump":             spireNcnCkdump,
				"ckdump_helper":      spireNcnCkdumpHelper,
				"cpsmount":           spireNcnCpsmount,
				"cpsmount_helper":    spireNcnCpsmountHelper,
				"dvs_hmi":            spireNcnDvsHmi,
				"dvs_map":            spireNcnDvsMap,
				"heartbeat":          spireNcnHeartbeat,
				"orca":               spireNcnOrca,
				"tpm_provisioner":    spireNcnTPMProvisioner,
			},
			"compute": map[string]interface{}{
				"cfs_state_reporter": spireComputeCfsStateReporter,
				"ckdump":             spireComputeCkdump,
				"ckdump_helper":      spireComputeCkdumpHelper,
				"cpsmount":           spireComputeCpsmount,
				"cpsmount_helper":    spireComputeCpsmountHelper,
				"dvs_hmi":            spireComputeDvsHmi,
				"dvs_map":            spireComputeDvsMap,
				"heartbeat":          spireComputeHeartbeat,
				"orca":               spireComputeOrca,
				"tpm_provisioner":    spireComputeTPMProvisioner,
				"wlm":                spireComputeWlm,
			},
		},
	}

	f, err = os.Create("test.rego")
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	fmt.Println("Rendering test template to 'test.rego'")
	err = tpl.Execute(f, values)
	if err != nil {
		log.Fatal(err)
	}

	f.Close()

	fmt.Println("Rendered test template:")
	fmt.Println("*****")

	dat, err = ioutil.ReadFile("test.rego")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s", string(dat))

	fmt.Println("*****")

	fmt.Printf("GET %s\n", ts.URL)
	res, err := http.Get(ts.URL)
	if err != nil {
		log.Fatal(err)
	}
	greeting, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s", greeting)
	fmt.Println("*****")
	fmt.Printf("Policy File: %s\n", policyTemplateFilename)
	fmt.Printf("Test File: %s\n", testTemplateFilename)

	fmt.Println("Executing ./opa_envoy_linux_amd64 check -S ./policy.rego ./test.rego")

	cmd := exec.Command("./opa_envoy_linux_amd64", "check", "-S", "./policy.rego")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Executing ./opa_envoy_linux_amd64 test ./policy.rego ./test.rego -v")

	cmd = exec.Command("./opa_envoy_linux_amd64", "test", "./policy.rego", "./test.rego", "-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}
