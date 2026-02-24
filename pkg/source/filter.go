package source

import "strings"

// DefaultAIKeywords is the base set used for filtering AI-related content.
var DefaultAIKeywords = []string{
	"artificial intelligence", "machine learning", "deep learning",
	"neural network", "LLM", "large language model", "GPT",
	"transformer", "diffusion", "stable diffusion", "midjourney",
	"computer vision", "NLP", "natural language processing",
	"generative AI", "gen AI", "genai",
	"AGI", "reinforcement learning", "fine-tuning", "fine tuning",
	"RAG", "retrieval augmented", "vector database", "embedding",
	"tokenizer", "inference", "AI agent", "agentic",
	"copilot", "chatbot", "foundation model",
	"llama", "mistral", "gemini", "openai", "anthropic", "claude",
	"hugging face", "huggingface", "pytorch", "tensorflow",
	"CUDA", "GPU", "TPU",
	"text-to-image", "text-to-video", "text-to-speech",
	"speech recognition", "object detection", "image generation",
	"prompt engineering", "AI safety", "alignment",
	"multimodal", "vision language model", "VLM",
	"AI coding", "code generation", "AI assistant",
}

// Filter holds keyword lists for AI content matching.
type Filter struct {
	keywords []string
	exclude  []string
}

// NewFilter creates a filter with default AI keywords plus extras.
func NewFilter(extraKeywords, excludeKeywords []string) *Filter {
	keywords := make([]string, len(DefaultAIKeywords))
	copy(keywords, DefaultAIKeywords)
	keywords = append(keywords, extraKeywords...)

	// Lowercase all keywords for case-insensitive matching.
	for i, kw := range keywords {
		keywords[i] = strings.ToLower(kw)
	}

	exclude := make([]string, len(excludeKeywords))
	for i, kw := range excludeKeywords {
		exclude[i] = strings.ToLower(kw)
	}

	return &Filter{keywords: keywords, exclude: exclude}
}

// MatchesAI returns true if text contains AI-related keywords.
func (f *Filter) MatchesAI(text string) bool {
	lower := strings.ToLower(text)

	for _, ex := range f.exclude {
		if strings.Contains(lower, ex) {
			return false
		}
	}

	for _, kw := range f.keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// MatchesAIDefault uses the default keyword list without extras.
func MatchesAIDefault(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range DefaultAIKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
