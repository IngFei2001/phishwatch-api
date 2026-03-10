package main

import (
	"database/sql"
	"log"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

var db *sql.DB

type GreyURL struct {
	ID         int    `json:"id"`
	URLPattern string `json:"url_pattern"`
	RiskScore  int    `json:"risk_score"`
}

func main() {
	// 连接 Neon 数据库
	var err error
	dbURL := "postgresql://neondb_owner:npg_MTkL9RNh1Zzl@ep-falling-haze-a1citxdd-pooler.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"

	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("数据库连接失败:", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("数据库 Ping 失败:", err)
	}
	log.Println("数据库连接成功!")

	r := gin.Default()

	r.GET("/api/check", checkURL)
	r.POST("/api/urls", addURL)
	r.PUT("/api/urls", updateURL) // 修改分数

	// Render 会提供 PORT 环境变量
	r.Run()
}

// normalizeURL 标准化 URL 格式
// 输入：https://example.com/suspicious、http://example.com/suspicious、example.com/suspicious
// 输出：example.com/suspicious（统一格式）
func normalizeURL(rawURL string) string {
	urlToParse := rawURL
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		urlToParse = "https://" + rawURL
	}

	u, err := url.Parse(urlToParse)
	if err != nil {
		log.Printf("URL normalization failed: %v, using original: %s", err, rawURL)
		return rawURL
	}

	// 统一格式：hostname + pathname（去除尾部斜杠）
	normalized := u.Hostname() + u.Path
	if strings.HasSuffix(normalized, "/") {
		normalized = strings.TrimSuffix(normalized, "/")
	}

	return normalized
}

// GET /api/check?url=xxx
func checkURL(c *gin.Context) {
	urlParam := c.Query("url")
	if urlParam == "" {
		c.JSON(400, gin.H{"error": "缺少 url 参数"})
		return
	}

	normalizedURL := normalizeURL(urlParam)

	var u GreyURL
	err := db.QueryRow("SELECT id, url_pattern, risk_score FROM grey_urls WHERE url_pattern = $1", normalizedURL).
		Scan(&u.ID, &u.URLPattern, &u.RiskScore)

	if err == sql.ErrNoRows {
		c.JSON(200, gin.H{"found": false, "url": normalizedURL, "message": "URL 不在灰名单中"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"found": true, "url": u.URLPattern, "risk_score": u.RiskScore})
}

// POST /api/urls - 添加新 URL
func addURL(c *gin.Context) {
	var input struct {
		URLPattern string `json:"url_pattern"`
		RiskScore  int    `json:"risk_score"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "无效的 JSON 数据"})
		return
	}

	if input.RiskScore < 8 || input.RiskScore > 10 {
		c.JSON(400, gin.H{"error": "分数必须 8-10 才能存入"})
		return
	}

	// 标准化 URL
	normalizedURL := normalizeURL(input.URLPattern)

	var existingID int
	err := db.QueryRow("SELECT id FROM grey_urls WHERE url_pattern = $1", normalizedURL).Scan(&existingID)
	if err == sql.ErrNoRows {
		var id int
		err := db.QueryRow("INSERT INTO grey_urls (url_pattern, risk_score) VALUES ($1, $2) RETURNING id",
			normalizedURL, input.RiskScore).Scan(&id)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, gin.H{"message": "URL 已添加", "id": id, "risk_score": input.RiskScore})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(400, gin.H{"message": "URL 已存在，如果要更新请使用 PUT 接口"})
}

// PUT /api/urls - 更新 URL 分数
func updateURL(c *gin.Context) {
	var input struct {
		URLPattern string `json:"url_pattern"`
		RiskScore  int    `json:"risk_score"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "无效的 JSON 数据"})
		return
	}

	if input.RiskScore < 8 || input.RiskScore > 10 {
		c.JSON(400, gin.H{"error": "分数必须 8-10"})
		return
	}

	normalizedURL := normalizeURL(input.URLPattern)

	var existingID int
	var oldScore int
	err := db.QueryRow("SELECT id, risk_score FROM grey_urls WHERE url_pattern = $1", normalizedURL).Scan(&existingID, &oldScore)
	if err == sql.ErrNoRows {
		c.JSON(404, gin.H{"error": "URL 不存在"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	_, err = db.Exec("UPDATE grey_urls SET risk_score = $1 WHERE id = $2", input.RiskScore, existingID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"message":   "分数已更新",
		"id":        existingID,
		"old_score": oldScore,
		"new_score": input.RiskScore,
	})
}


我这里是不是有关于UPDATE的api
