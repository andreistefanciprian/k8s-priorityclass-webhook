package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const (
	jsonContentType   = `application/json`
	priorityClassName = "high-priority-nonpreempting"
)

var (
	deserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
)

func main() {
	// Parse CLI params
	parameters := parseFlags()

	// Create a new https server
	httpsMux := mux.NewRouter()

	// priorityClassName handler
	httpsMux.HandleFunc("/mutate", HandlePriorityClass)

	httpsAddr := ":" + strconv.Itoa(parameters.httpsPort)
	httpsServer := http.Server{
		Addr:    httpsAddr,
		Handler: httpsMux,
	}

	// Start the https server
	log.Printf("Starting https Server on port %s", httpsAddr)
	err := httpsServer.ListenAndServeTLS(parameters.certFile, parameters.keyFile)
	if err != nil {
		log.Fatal(err)
	}
}

// ServerParameters struct holds the parameters for the webhook server.
type ServerParameters struct {
	httpsPort int    // https server port
	certFile  string // path to the x509 certificate for https
	keyFile   string // path to the x509 private key matching `CertFile`
}

// patchOperation is a JSON patch operation, see https://jsonpatch.com/
type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// parseFlags parses the CLI params and returns a ServerParameters struct.
func parseFlags() ServerParameters {
	var parameters ServerParameters

	// Define and parse CLI params using the "flag" package.
	flag.IntVar(&parameters.httpsPort, "httpsPort", 443, " Https server port (webhook endpoint).")
	flag.StringVar(&parameters.certFile, "tlsCertFile", "/etc/webhook/certs/tls.crt", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&parameters.keyFile, "tlsKeyFile", "/etc/webhook/certs/tls.key", "File containing the x509 private key to --tlsCertFile.")
	flag.Parse()

	return parameters
}

// HandlePriorityClass is the HTTP handler function for the /mutate endpoint.
func HandlePriorityClass(w http.ResponseWriter, r *http.Request) {

	// Validate Request (Valid requests are POST with Content-Type: application/json)
	if !validateRequest(w, r) {
		return
	}

	// Parse the AdmissionReview request and return it.
	admissionReviewReq, err := parseRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build AdmissionReview response.
	admissionReviewResponse, err := buildResponse(w, *admissionReviewReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the AdmissionReview response to the http response writer.
	sendResponse(w, *admissionReviewResponse)
}

// validateRequest checks requests are POST with Content-Type: application/json
func validateRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return false
	}

	if contentType := r.Header.Get("Content-Type"); contentType != jsonContentType {
		http.Error(w, fmt.Sprintf("Invalid content type %s", contentType), http.StatusBadRequest)
		return false
	}

	return true
}

// parseRequest parses the AdmissionReview request.
func parseRequest(w http.ResponseWriter, r *http.Request) (*v1beta1.AdmissionReview, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %s", err.Error())
	}

	var admissionReviewReq v1beta1.AdmissionReview
	if _, _, err := deserializer.Decode(body, nil, &admissionReviewReq); err != nil {
		return nil, fmt.Errorf("could not deserialize request: %s", err.Error())
	} else if admissionReviewReq.Request == nil {
		return nil, fmt.Errorf("malformed admission review (request is nil)")
	}

	return &admissionReviewReq, nil
}

// buildResponse builds the AdmissionReview response.
func buildResponse(w http.ResponseWriter, req v1beta1.AdmissionReview) (*v1beta1.AdmissionReview, error) {

	// Unmarshal the Deployment object from the AdmissionReview request into a Deployment struct.
	deployment := v1.Deployment{}
	err := json.Unmarshal(req.Request.Object.Raw, &deployment)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal pod on admission request: %s", err.Error())
	}

	// Construct Deployment name in the format: namespace/name
	deploymentName := deployment.GetNamespace() + "/" + deployment.GetName()

	log.Printf("New Admission Review Request is being processed: User: %v \t Operation: %v \t Deployment: %v \n",
		req.Request.UserInfo.Username,
		req.Request.Operation,
		deploymentName,
	)
	// Print string(body) when you want to see the AdmissionReview in the logs
	// log.Printf("Admission Request Body: \n %v", string(body))

	// Construct the AdmissionReview response.
	admissionReviewResponse := v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{
			UID:     req.Request.UID,
			Allowed: true,
		},
	}

	//  Check if priorityClassName is already set
	if deployment.Spec.Template.Spec.PriorityClassName == priorityClassName {
		log.Printf("Deployment %v has PriorityClassName already set to: %v", deploymentName, deployment.Spec.Template.Spec.PriorityClassName)
	} else {
		patchBytes, err := buildJsonPatch(priorityClassName)
		if err != nil {
			return nil, fmt.Errorf("could not build JSON patch: %s", err.Error())
		}
		admissionReviewResponse.Response.AuditAnnotations = deployment.ObjectMeta.Annotations // AuditAnnotations are added to the audit record when this admission response is added to the audit event.
		admissionReviewResponse.Response.Patch = patchBytes
		patchMsg := fmt.Sprintf("PriorityClassName %v added to Deployment %v.", priorityClassName, deploymentName)

		if deployment.Spec.Template.Spec.PriorityClassName == "" {
			stdoutMsg := fmt.Sprintf("Deployment %v does not have a PriorityClassName set.", deploymentName)
			log.Println(stdoutMsg, patchMsg)
			admissionReviewResponse.Response.Warnings = []string{stdoutMsg, patchMsg}

		} else {
			stdoutMsg := fmt.Sprintf("Deployment %v has PriorityClassName already set to: %v",
				deploymentName,
				deployment.Spec.Template.Spec.PriorityClassName,
			)
			log.Println(stdoutMsg, patchMsg)
			admissionReviewResponse.Response.Warnings = []string{stdoutMsg, patchMsg}
		}
	}

	return &admissionReviewResponse, nil
}

// sendResponse writes the AdmissionReview response to the http response writer.
func sendResponse(w http.ResponseWriter, admissionReviewResponse v1beta1.AdmissionReview) {
	// Marshal the AdmissionReview response to JSON.
	bytes, err := json.Marshal(&admissionReviewResponse)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not marshal JSON Admission Response: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Write the AdmissionReview response to the http response writer.
	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}

// buildJsonPatch builds a JSON patch to add the priorityClassName and annotation to a Deployment.
func buildJsonPatch(priorityClassName string) ([]byte, error) {
	now := time.Now()
	annotation := make(map[string]string)
	annotation["priorityClassWebhook/updated_at"] = now.Format("Mon Jan 2 15:04:05 AEST 2006")
	patch := []patchOperation{
		patchOperation{
			Op:    "add",
			Path:  "/spec/template/spec/priorityClassName",
			Value: priorityClassName,
		},
		patchOperation{
			Op:    "add",
			Path:  "/metadata/annotations",
			Value: annotation,
		},
	}
	// Marshal the patch slice to JSON.
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON patch: %s", err.Error())
	}

	return patchBytes, nil
}
