package server

// mergeGeminiAFCIntoGenerationConfig is a no-op for the REST API.
// automaticFunctionCallingConfig is a client-SDK-only concept and is not
// accepted by the Generative Language v1beta REST endpoint. AFC does not
// apply to raw HTTP calls since tools must be explicitly passed in the
// request payload — there is no implicit function calling via REST.
func mergeGeminiAFCIntoGenerationConfig(generationConfig map[string]interface{}) {
	// intentionally empty — AFC is SDK-only, not a REST field
}
