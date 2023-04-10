/*
Nexodus API

This is the Nexodus API Server.

API version: 1.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package public

// ModelsDeviceStartResponse struct for ModelsDeviceStartResponse
type ModelsDeviceStartResponse struct {
	ClientId string `json:"client_id,omitempty"`
	// TODO: Remove this once golang/oauth2 supports device flow and when coreos/go-oidc adds device_authorization_endpoint discovery
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint,omitempty"`
	Issuer                      string `json:"issuer,omitempty"`
}