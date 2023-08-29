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
	jsonContentType = `application/json`
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

	// Mutate the Deployment object and return the AdmissionReview response.
	bytes, err := mutateDeployment(w, *admissionReviewReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the AdmissionReview response to the http response writer.
	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}

// HandleHealthz is a liveness probe.
func HandleHealthz(w http.ResponseWriter, r *http.Request) {
	log.Printf("Health check at %v\n", r.URL.Path)
	w.WriteHeader(http.StatusOK)
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

// mutateDeployment mutates the Deployment object and returns the AdmissionReview response.
func mutateDeployment(w http.ResponseWriter, req v1beta1.AdmissionReview) ([]byte, error) {

	// Unmarshal the Deployment object from the AdmissionReview request into a Deployment struct.
	deployment := v1.Deployment{}
	err := json.Unmarshal(req.Request.Object.Raw, &deployment)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal pod on admission request: %s", err.Error())
	}

	// Construct Deployment name in the format: namespace/name
	deploymentName := deployment.GetNamespace() + "/" + deployment.GetName()

	log.Printf("New Admission Review Request is being processed: User: %v \t Deployment: %v \n",
		req.Request.UserInfo.Username,
		deploymentName,
	)
	// Print string(body) when you want to see the AdmissionReview in the logs
	// log.Printf("Admission Request Body: \n %v", string(body))

	//  Check if priorityClassName is already set
	if deployment.Spec.Template.Spec.PriorityClassName != "" {
		log.Printf("Deployment %v has PriorityClassName already set to: %v",
			deploymentName,
			deployment.Spec.Template.Spec.PriorityClassName,
		)
	} else {
		log.Printf("Deployment %v does not have PriorityClassName set.", deploymentName)
	}

	// Construct the JSON patch operation for adding the "priorityClassName" field to the pod spec.
	priorityClassName_patch := patchOperation{
		Op:    "add",
		Path:  "/spec/template/spec/priorityClassName",
		Value: "high-priority-nonpreempting",
	}

	// Construct the JSON patch operation for adding the "priorityClassWebhook/updated_at" annotation to the pod metadata.
	now := time.Now()
	deployment.ObjectMeta.Annotations["priorityClassWebhook/updated_at"] = now.Format("Mon Jan 2 15:04:05 AEST 2006")
	annotation_patch := patchOperation{
		Op:    "add",
		Path:  "/metadata/annotations",
		Value: deployment.ObjectMeta.Annotations,
	}

	// Append the patch operations to the patches slice.
	var patches []patchOperation
	patches = append(patches, priorityClassName_patch, annotation_patch)

	// Marshal the patches slice to JSON.
	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON patch: %s", err.Error())
	}

	patch_msg := fmt.Sprintf("PriorityClassName %v added to Deployment %v.", priorityClassName_patch.Value, deploymentName)

	// Construct the AdmissionReview response.
	admissionReviewResponse := v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{
			UID:     req.Request.UID,
			Allowed: true,
			// Result:           &metav1.Status{Message: patch_msg},	// Result contains extra details into why an admission request was denied.
			AuditAnnotations: deployment.ObjectMeta.Annotations, // AuditAnnotations are added to the audit record when this admission response is added to the audit event.
			Warnings:         []string{patch_msg},
		},
	}
	admissionReviewResponse.Response.Patch = patchBytes

	// Send the AdmissionReview response.
	bytes, err := json.Marshal(&admissionReviewResponse)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON Admission Response: %s", err.Error())
	}

	log.Println(patch_msg)
	return bytes, nil
}
