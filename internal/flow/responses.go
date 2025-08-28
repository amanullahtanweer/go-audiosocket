package flow

import (
	"strings"
)

// ResponseType represents the classification of a user response
type ResponseType string

const (
	ResponsePositive ResponseType = "positive"
	ResponseNegative ResponseType = "negative"
	ResponseUnknown  ResponseType = "unknown"
)

// ResponseClassifier classifies user responses based on keywords
type ResponseClassifier struct {
	positiveKeywords []string
	negativeKeywords []string
}

// NewResponseClassifier creates a new response classifier
func NewResponseClassifier() *ResponseClassifier {
	return &ResponseClassifier{
		positiveKeywords: []string{
			"yes", "yeah", "i have", "already have", "got one", "enrolled", 
			"both parts", "part a", "part b", "have it", "i do", "sure",
			"maybe", "i think so", "probably", "i believe so",
		},
		negativeKeywords: []string{
			"no", "don't have", "do not have", "not yet", "no coverage", 
			"i don't", "i don't think so", "negative", "nope", "nah",
			"i don't want", "not interested", "leave me alone",
		},
	}
}

// ClassifyResponse classifies a user response as positive, negative, or unknown
func (rc *ResponseClassifier) ClassifyResponse(text string) ResponseType {
	text = strings.ToLower(strings.TrimSpace(text))
	
	// Check for negative keywords first (to avoid false positives)
	for _, keyword := range rc.negativeKeywords {
		if strings.Contains(text, keyword) {
			return ResponseNegative
		}
	}
	
	// Check for positive keywords
	for _, keyword := range rc.positiveKeywords {
		if strings.Contains(text, keyword) {
			return ResponsePositive
		}
	}
	
	// If no clear positive or negative keywords found, classify as unknown
	return ResponseUnknown
}

// GetPositiveKeywords returns the list of positive keywords
func (rc *ResponseClassifier) GetPositiveKeywords() []string {
	return rc.positiveKeywords
}

// GetNegativeKeywords returns the list of negative keywords
func (rc *ResponseClassifier) GetNegativeKeywords() []string {
	return rc.negativeKeywords
}

// AddPositiveKeyword adds a new positive keyword
func (rc *ResponseClassifier) AddPositiveKeyword(keyword string) {
	rc.positiveKeywords = append(rc.positiveKeywords, strings.ToLower(keyword))
}

// AddNegativeKeyword adds a new negative keyword
func (rc *ResponseClassifier) AddNegativeKeyword(keyword string) {
	rc.negativeKeywords = append(rc.negativeKeywords, strings.ToLower(keyword))
}

// RemovePositiveKeyword removes a positive keyword
func (rc *ResponseClassifier) RemovePositiveKeyword(keyword string) {
	keyword = strings.ToLower(keyword)
	for i, k := range rc.positiveKeywords {
		if k == keyword {
			rc.positiveKeywords = append(rc.positiveKeywords[:i], rc.positiveKeywords[i+1:]...)
			break
		}
	}
}

// RemoveNegativeKeyword removes a negative keyword
func (rc *ResponseClassifier) RemoveNegativeKeyword(keyword string) {
	keyword = strings.ToLower(keyword)
	for i, k := range rc.negativeKeywords {
		if k == keyword {
			rc.negativeKeywords = append(rc.negativeKeywords[:i], rc.negativeKeywords[i+1:]...)
			break
		}
	}
}
