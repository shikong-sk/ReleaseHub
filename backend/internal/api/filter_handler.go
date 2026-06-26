package api

import (
	"net/http"

	"releasehub/backend/internal/services/filter"

	"github.com/gin-gonic/gin"
)

type filterPreviewInput struct {
	AssetNames          []string `json:"assetNames"`
	FilterMode          string   `json:"filterMode"`
	IncludePatterns     string   `json:"includePatterns"`
	ExcludePatterns     string   `json:"excludePatterns"`
}

type filterPreviewResult struct {
	Name    string `json:"name"`
	Matched bool   `json:"matched"`
}

type filterPreviewResponse struct {
	Results []filterPreviewResult `json:"results"`
	Error   string                `json:"error,omitempty"`
}

func registerFilterRoutes(router *gin.Engine) {
	router.POST("/api/filter/preview", filterPreview)
}

func filterPreview(c *gin.Context) {
	var input filterPreviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	if len(input.AssetNames) == 0 {
		c.JSON(http.StatusOK, filterPreviewResponse{Results: []filterPreviewResult{}})
		return
	}

	matcher, err := filter.NewMatcher(input.FilterMode, input.IncludePatterns, input.ExcludePatterns)
	if err != nil {
		c.JSON(http.StatusOK, filterPreviewResponse{
			Error: "过滤规则无效: " + err.Error(),
		})
		return
	}

	results := make([]filterPreviewResult, 0, len(input.AssetNames))
	for _, name := range input.AssetNames {
		matched, matchErr := matcher.Match(name)
		if matchErr != nil {
			results = append(results, filterPreviewResult{Name: name, Matched: false})
			continue
		}
		results = append(results, filterPreviewResult{Name: name, Matched: matched})
	}

	c.JSON(http.StatusOK, filterPreviewResponse{Results: results})
}
