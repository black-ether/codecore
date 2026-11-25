package main

type rankingTerm struct {
	Term  string  `json:"term"`
	TF    int     `json:"tf"`
	DF    int     `json:"df"`
	IDF   float64 `json:"idf"`
	BM25  float64 `json:"bm25"`
	Bonus float64 `json:"bonus"`
}

type rankingExplanation struct {
	Adjustment      float64       `json:"adjustment,omitempty"`
	SupportingTerms []string      `json:"supportingTerms,omitempty"`
	DocumentLength  int           `json:"documentLength"`
	MeanLength      float64       `json:"meanLength"`
	Documents       int           `json:"documents"`
	DefinitionTerms []string      `json:"definitionTerms,omitempty"`
	Covered         int           `json:"covered"`
	Terms           []rankingTerm `json:"terms"`
}
