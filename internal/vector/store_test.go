package vector

import (
	"testing"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float64{1, 0, 0}
	score := CosineSimilarity(v, v)
	if score != 1.0 {
		t.Errorf("CosineSimilarity(identical) = %f, want 1.0", score)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{0, 1, 0}
	score := CosineSimilarity(a, b)
	if score != 0.0 {
		t.Errorf("CosineSimilarity(orthogonal) = %f, want 0.0", score)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{-1, 0, 0}
	score := CosineSimilarity(a, b)
	if score != -1.0 {
		t.Errorf("CosineSimilarity(opposite) = %f, want -1.0", score)
	}
}

func TestCosineSimilarity_Similar(t *testing.T) {
	a := []float64{1, 1, 0}
	b := []float64{1, 0.9, 0}
	score := CosineSimilarity(a, b)
	if score < 0.9 || score > 1.0 {
		t.Errorf("CosineSimilarity(similar) = %f, want ~0.99", score)
	}
}

func TestCosineSimilarity_DifferentDimensions(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{1, 0}
	score := CosineSimilarity(a, b)
	if score != 0 {
		t.Errorf("CosineSimilarity(different dimensions) = %f, want 0", score)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float64{0, 0, 0}
	b := []float64{1, 0, 0}
	score := CosineSimilarity(a, b)
	if score != 0 {
		t.Errorf("CosineSimilarity(zero vector) = %f, want 0", score)
	}
}

func TestValidateDimension(t *testing.T) {
	if !ValidateDimension([]float64{1, 2, 3}, 3) {
		t.Error("ValidateDimension should return true for matching dimension")
	}
	if ValidateDimension([]float64{1, 2}, 3) {
		t.Error("ValidateDimension should return false for mismatched dimension")
	}
}

func TestDefaultSearchOptions(t *testing.T) {
	opts := DefaultSearchOptions()
	if opts.TopK != 10 {
		t.Errorf("Default TopK = %d, want 10", opts.TopK)
	}
	if opts.MinScore != 0.5 {
		t.Errorf("Default MinScore = %f, want 0.5", opts.MinScore)
	}
}