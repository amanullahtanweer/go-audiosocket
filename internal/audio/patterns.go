package audio

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// InterruptConfig represents the configuration structure
type InterruptConfig struct {
	Interrupts map[string]InterruptRule `yaml:"interrupts"`
	Settings   Settings                 `yaml:"settings"`
}

// InterruptRule represents a single interrupt rule
type InterruptRule struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	AudioFile   string    `yaml:"audio_file"`
	Priority    int       `yaml:"priority"`
	Patterns    []Pattern `yaml:"patterns"`
}

// Pattern represents a single pattern to match
type Pattern struct {
	Type          string     `yaml:"type"`
	Phrases       []string   `yaml:"phrases,omitempty"`
	Words         [][]string `yaml:"words,omitempty"`
	RequiredWords [][]string `yaml:"required_words,omitempty"`
	WordGroups    [][]string `yaml:"word_groups,omitempty"`
}

// Settings represents pattern matching settings
type Settings struct {
	CaseSensitive     bool `yaml:"case_sensitive"`
	PartialWordMatch  bool `yaml:"partial_word_match"`
	MaxWordsBetween   int  `yaml:"max_words_between"`
	ReloadOnDetection bool `yaml:"reload_on_detection"`
}

// PatternMatcher handles pattern matching for interrupts
type PatternMatcher struct {
	configPath string
	config     *InterruptConfig
	mu         sync.RWMutex
	lastLoad   time.Time
}

// NewPatternMatcher creates a new pattern matcher
func NewPatternMatcher(configPath string) (*PatternMatcher, error) {
	matcher := &PatternMatcher{
		configPath: configPath,
	}

	if err := matcher.loadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return matcher, nil
}

// loadConfig loads the configuration from file
func (matcher *PatternMatcher) loadConfig() error {
	matcher.mu.Lock()
	defer matcher.mu.Unlock()

	data, err := ioutil.ReadFile(matcher.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config InterruptConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	matcher.config = &config
	matcher.lastLoad = time.Now()

	log.Printf("Loaded interrupt config with %d rules", len(config.Interrupts))
	return nil
}

// reloadConfigIfNeeded reloads config if reload_on_detection is enabled
func (matcher *PatternMatcher) reloadConfigIfNeeded() error {
	matcher.mu.RLock()
	shouldReload := matcher.config.Settings.ReloadOnDetection
	matcher.mu.RUnlock()

	if shouldReload {
		// Check if file has been modified
		fileInfo, err := os.Stat(matcher.configPath)
		if err != nil {
			return err
		}

		if fileInfo.ModTime().After(matcher.lastLoad) {
			log.Printf("Config file modified, reloading...")
			return matcher.loadConfig()
		}
	}

	return nil
}

// DetectInterrupt detects interrupts based on the given text
func (matcher *PatternMatcher) DetectInterrupt(text string) *InterruptRule {
	// Reload config if needed
	if err := matcher.reloadConfigIfNeeded(); err != nil {
		log.Printf("Failed to reload config: %v", err)
	}

	matcher.mu.RLock()
	defer matcher.mu.RUnlock()

	// Convert text based on settings
	searchText := text
	if !matcher.config.Settings.CaseSensitive {
		searchText = strings.ToLower(text)
	}

	// Check each interrupt rule in priority order
	for _, rule := range matcher.config.Interrupts {
		if matcher.matchesRule(searchText, rule) {
			log.Printf("Pattern match found: %s - '%s'", rule.Name, text)
			return &rule
		}
	}

	return nil
}

// matchesRule checks if the text matches any pattern in the rule
func (matcher *PatternMatcher) matchesRule(searchText string, rule InterruptRule) bool {
	for _, pattern := range rule.Patterns {
		if matcher.matchesPattern(searchText, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern checks if the text matches a specific pattern
func (matcher *PatternMatcher) matchesPattern(searchText string, pattern Pattern) bool {
	switch pattern.Type {
	case "exact":
		return matcher.matchesExact(searchText, pattern.Phrases)
	case "combo":
		return matcher.matchesCombo(searchText, pattern.Words)
	case "required":
		return matcher.matchesRequired(searchText, pattern.RequiredWords)
	case "alternative":
		return matcher.matchesAlternative(searchText, pattern.WordGroups)
	default:
		log.Printf("Unknown pattern type: %s", pattern.Type)
		return false
	}
}

// matchesExact checks for exact phrase matches
func (matcher *PatternMatcher) matchesExact(searchText string, phrases []string) bool {
	for _, phrase := range phrases {
		checkPhrase := phrase
		if !matcher.config.Settings.CaseSensitive {
			checkPhrase = strings.ToLower(phrase)
		}
		if strings.Contains(searchText, checkPhrase) {
			return true
		}
	}
	return false
}

// matchesCombo checks if ALL words in a combination are present
func (matcher *PatternMatcher) matchesCombo(searchText string, wordLists [][]string) bool {
	for _, wordList := range wordLists {
		allWordsPresent := true
		for _, word := range wordList {
			checkWord := word
			if !matcher.config.Settings.CaseSensitive {
				checkWord = strings.ToLower(word)
			}
			if !strings.Contains(searchText, checkWord) {
				allWordsPresent = false
				break
			}
		}
		if allWordsPresent {
			return true
		}
	}
	return false
}

// matchesRequired checks if ALL required word groups are present
func (matcher *PatternMatcher) matchesRequired(searchText string, requiredGroups [][]string) bool {
	words := strings.Fields(searchText)

	for _, group := range requiredGroups {
		groupMatched := false
		for _, requiredWord := range group {
			checkWord := requiredWord
			if !matcher.config.Settings.CaseSensitive {
				checkWord = strings.ToLower(requiredWord)
			}

			// Check if any word in the text matches this required word
			for _, word := range words {
				if strings.Contains(strings.ToLower(word), checkWord) {
					groupMatched = true
					break
				}
			}
			if groupMatched {
				break
			}
		}
		if !groupMatched {
			return false
		}
	}
	return true
}

// matchesAlternative checks if any word from each group is present
func (matcher *PatternMatcher) matchesAlternative(searchText string, wordGroups [][]string) bool {
	words := strings.Fields(searchText)

	for _, group := range wordGroups {
		groupMatched := false
		for _, alternativeWord := range group {
			checkWord := alternativeWord
			if !matcher.config.Settings.CaseSensitive {
				checkWord = strings.ToLower(alternativeWord)
			}

			// Check if any word in the text matches this alternative
			for _, word := range words {
				if strings.Contains(strings.ToLower(word), checkWord) {
					groupMatched = true
					break
				}
			}
			if groupMatched {
				break
			}
		}
		if !groupMatched {
			return false
		}
	}
	return true
}

// GetInterrupts returns all configured interrupts
func (matcher *PatternMatcher) GetInterrupts() map[string]InterruptRule {
	matcher.mu.RLock()
	defer matcher.mu.RUnlock()

	result := make(map[string]InterruptRule)
	for k, v := range matcher.config.Interrupts {
		result[k] = v
	}
	return result
}
