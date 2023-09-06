package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWebhookHandler tests the webhookHandler function.
func TestWebhookHandler(t *testing.T) {
	testCases := []struct {
		description      string
		request          string
		expectedStatus   int
		expectedResponse string
	}{
		{
			description:      "CREATE Deployment priorityClassName not set",
			request:          makeAdmissionRequest("Deployment", "CREATE", "foo/test-deployment", ""),
			expectedStatus:   http.StatusOK,
			expectedResponse: `{"response":{"uid":"f0b23c24-35f6-42a3-99e3-aa4ccab85f91","allowed":true,"patch":"W3sib3AiOiJhZGQiLCJwYXRoIjoiL3NwZWMvdGVtcGxhdGUvc3BlYy9wcmlvcml0eUNsYXNzTmFtZSIsInZhbHVlIjoiaGlnaC1wcmlvcml0eS1ub25wcmVlbXB0aW5nIn0seyJvcCI6InJlcGxhY2UiLCJwYXRoIjoiL21ldGFkYXRhL2Fubm90YXRpb25zIiwidmFsdWUiOnsic29tZV9hbm5vdGF0aW9uIjoic29tZV92YWx1ZSIsInVwZGF0ZWRfYnkiOiJwcmlvcml0eUNsYXNzV2ViaG9vayJ9fV0=","warnings":["Deployment foo/test-deployment does not have a PriorityClassName set.","Deployment foo/test-deployment was updated with PriorityClassName high-priority-nonpreempting."]}}`,
		},
		{
			description:      "CREATE Deployment priorityClassName set to different class",
			request:          makeAdmissionRequest("Deployment", "CREATE", "foo/test-deployment", "some-priority-class"),
			expectedStatus:   http.StatusOK,
			expectedResponse: `{"response":{"uid":"f0b23c24-35f6-42a3-99e3-aa4ccab85f91","allowed":true,"patch":"W3sib3AiOiJhZGQiLCJwYXRoIjoiL3NwZWMvdGVtcGxhdGUvc3BlYy9wcmlvcml0eUNsYXNzTmFtZSIsInZhbHVlIjoiaGlnaC1wcmlvcml0eS1ub25wcmVlbXB0aW5nIn0seyJvcCI6InJlcGxhY2UiLCJwYXRoIjoiL21ldGFkYXRhL2Fubm90YXRpb25zIiwidmFsdWUiOnsic29tZV9hbm5vdGF0aW9uIjoic29tZV92YWx1ZSIsInVwZGF0ZWRfYnkiOiJwcmlvcml0eUNsYXNzV2ViaG9vayJ9fV0=","warnings":["Deployment foo/test-deployment has PriorityClassName already set to: some-priority-class","Deployment foo/test-deployment was updated with PriorityClassName high-priority-nonpreempting."]}}`,
		},
		{
			description:      "UPDATE Deployment priorityClassName set to target class",
			request:          makeAdmissionRequest("Deployment", "UPDATE", "foo/test-deployment", "high-priority-nonpreempting"),
			expectedStatus:   http.StatusOK,
			expectedResponse: `{"response":{"uid":"f0b23c24-35f6-42a3-99e3-aa4ccab85f91","allowed":true}}`,
		},
		{
			description:      "CREATE Deployment priorityClassName set to target class",
			request:          makeAdmissionRequest("Deployment", "CREATE", "foo/test-deployment", "high-priority-nonpreempting"),
			expectedStatus:   http.StatusOK,
			expectedResponse: `{"response":{"uid":"f0b23c24-35f6-42a3-99e3-aa4ccab85f91","allowed":true}}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			req := bytes.NewBufferString(testCase.request)

			server := httptest.NewServer(http.HandlerFunc(webhookHandler))
			defer server.Close()
			resp, err := http.Post(server.URL, jsonContentType, req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != testCase.expectedStatus {
				t.Errorf("Expected status code %d, got %d", testCase.expectedStatus, resp.StatusCode)
			}
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != testCase.expectedResponse {
				t.Errorf("Expected response body %s, got %s", testCase.expectedResponse, string(data))
			}
		})
	}
}

// makeAdmissionRequest is a helper function to create an AdmissionReview request
func makeAdmissionRequest(k8sObjectKind, k8sApiEvent, k8sObjectFullName, priorityClassName string) string {
	k8sObjectNamespace, k8sObjectName := strings.Split(k8sObjectFullName, "/")[0], strings.Split(k8sObjectFullName, "/")[1]
	k8sObect := fmt.Sprintf(
		`{
			"kind": "AdmissionReview",
			"apiVersion": "admission.k8s.io/v1beta1",
			"request": {
			  "uid": "f0b23c24-35f6-42a3-99e3-aa4ccab85f91",
			  "kind": {
				"group": "apps",
				"version": "v1",
				"kind": "%s"
			  },
			  "operation": "%s",
			  "userInfo": {
				"username": "someuser@gmail.com"
			  },
			  "object": {
				"kind": "%s",
				"apiVersion": "apps/v1",
				"metadata": {
				  "name": "%s",
				  "namespace": "%s",
				  "annotations": {
					"some_annotation": "some_value"
				  }
				},
				%s
			  }
			}
		  }`,
		k8sObjectKind,
		k8sApiEvent,
		k8sObjectKind,
		k8sObjectName,
		k8sObjectNamespace,
		getPriorityClassPodSpec(priorityClassName),
	)
	return k8sObect
}

// getPriorityClassPodSpec is a helper function to create a pod spec with a priorityClassName
func getPriorityClassPodSpec(priorityClassName string) string {
	if priorityClassName == "" {
		return `"spec": {"template": {"spec": {"restartPolicy": "Always"}}}`
	} else {
		return fmt.Sprintf(
			`"spec": {"template": {"spec": {"restartPolicy": "Always", "priorityClassName": "%s"}}}`,
			priorityClassName,
		)
	}
}
