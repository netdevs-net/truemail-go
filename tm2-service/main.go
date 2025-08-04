package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"github.com/miekg/dns"
	"net/smtp"
	"github.com/emersion/go-imap/client"
)

// TM2 Configuration
const (
	SMTP_TIMEOUT     = 2 * time.Second
	IMAP_TIMEOUT     = 3 * time.Second
	DNS_TIMEOUT      = 2 * time.Second
	CONNECTION_POOL_SIZE = 10
	WORKER_POOL_SIZE = 20
	BULK_BATCH_SIZE  = 50
)

// Global variables for TM2 optimizations
var (
	// Connection pools
	dnsClientPool = make(chan *dns.Client, CONNECTION_POOL_SIZE)
	smtpClientPool = make(chan *smtp.Client, CONNECTION_POOL_SIZE)
	
	// DNS cache for MX records
	mxCache = make(map[string][]string)
	mxCacheMutex sync.RWMutex
	mxCacheTTL = 5 * time.Minute
	
	// Domain cache for validation results
	domainCache = make(map[string]DomainValidationResult)
	domainCacheMutex sync.RWMutex
	domainCacheTTL = 10 * time.Minute
	
	// Worker pool for bulk operations
	workerPool = make(chan struct{}, WORKER_POOL_SIZE)
	
	// Rate limiting with improved algorithm
	rateLimiter = make(map[string]time.Time)
	rateMutex   sync.RWMutex
	
	// Structured logging
	logger = log.New(log.Writer(), "[TM2] ", log.LstdFlags)
	
	// Major email providers for smart validation
	majorProviders = map[string]bool{
		"gmail.com": true,
		"yahoo.com": true,
		"outlook.com": true,
		"hotmail.com": true,
		"live.com": true,
		"icloud.com": true,
		"aol.com": true,
		"protonmail.com": true,
	}
	
	// Disposable email domains
	disposableDomains = map[string]bool{
		"10minutemail.com": true,
		"mailinator.com":   true,
		"guerrillamail.com": true,
		"tempmail.org":     true,
		"example.com":      true,
		"test.com":         true,
		"example.org":      true,
		"example.net":      true,
		"yopmail.com":      true,
		"mailnesia.com":    true,
		"sharklasers.com":  true,
		"getairmail.com":   true,
		"maildrop.cc":      true,
		"mailinator2.com":  true,
		"spam4.me":         true,
		"bccto.me":         true,
		"chacuo.net":       true,
		"dispostable.com":  true,
		"fakeinbox.com":    true,
		"guerrillamailblock.com": true,
	}

	// Role-based email patterns
	rolePatterns = []string{
		"admin", "info", "contact", "support", "help", "sales", "marketing",
		"noreply", "no-reply", "donotreply", "do-not-reply", "webmaster",
		"postmaster", "abuse", "security", "billing", "accounts",
	}
)

// TM2 Data Structures
type ValidationRequest struct {
	Email string `json:"email"`
}

type ValidationResponse struct {
	Email       string    `json:"email"`
	Status      string    `json:"status"`
	Score       int       `json:"score"`
	Details     string    `json:"details"`
	ValidatedAt time.Time `json:"validated_at"`
	ValidationMethod string `json:"validation_method"`
}

type BulkValidationRequest struct {
	Emails []string `json:"emails"`
	Quiet  bool     `json:"quiet"` // Reduce logging noise
}

type BulkValidationResponse struct {
	Results []ValidationResponse `json:"results"`
	Stats   BulkStats           `json:"stats"`
}

type BulkStats struct {
	Total     int     `json:"total"`
	Valid     int     `json:"valid"`
	Invalid   int     `json:"invalid"`
	Unknown   int     `json:"unknown"`
	RateLimited int   `json:"rate_limited"`
	Duration  float64 `json:"duration_seconds"`
	Speed     float64 `json:"emails_per_second"`
}

type HealthResponse struct {
	Status    string    `json:"status"`
	Service   string    `json:"service"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Stats     HealthStats `json:"stats"`
}

type HealthStats struct {
	CacheSize     int `json:"cache_size"`
	WorkerPoolSize int `json:"worker_pool_size"`
	ConnectionPoolSize int `json:"connection_pool_size"`
}

type DomainValidationResult struct {
	IsValid    bool
	HasMX      bool
	MXServers  []string
	ValidatedAt time.Time
}

type EmailValidationResult struct {
	IsValidFormat     bool
	IsDisposable      bool
	IsRoleBased       bool
	IsValidDomain     bool
	HasMXRecords      bool
	IsValidSMTP       bool
	IsValidIMAP       bool
	ValidationSteps   []string
	Details           string
	Duration          time.Duration
}

func main() {
	port := "8080"
	
	logger.Printf("🚀 TM2 (TrueMail 2.0) starting on port %s", port)
	logger.Printf("⚡ Optimizations: Connection pooling, DNS caching, Worker pools, Smart validation")
	
	// Initialize connection pools
	initializeConnectionPools()
	
	// Start cache cleanup goroutine
	go cacheCleanupRoutine()
	
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/validate", handleSingleValidation)
	http.HandleFunc("/validate/bulk", handleBulkValidation)
	
	logger.Fatal(http.ListenAndServe(":"+port, nil))
}

func initializeConnectionPools() {
	// Initialize DNS client pool
	for i := 0; i < CONNECTION_POOL_SIZE; i++ {
		dnsClient := &dns.Client{Timeout: DNS_TIMEOUT}
		dnsClientPool <- dnsClient
	}
	
	logger.Printf("✅ Connection pools initialized: DNS=%d, SMTP=%d, Workers=%d", 
		CONNECTION_POOL_SIZE, CONNECTION_POOL_SIZE, WORKER_POOL_SIZE)
}

func cacheCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		cleanupCaches()
	}
}

func cleanupCaches() {
	now := time.Now()
	
	// Cleanup MX cache (simplified - no TTL for now)
	mxCacheMutex.Lock()
	// For now, we'll keep MX cache entries indefinitely
	// TODO: Implement proper TTL for MX cache if needed
	mxCacheMutex.Unlock()
	
	// Cleanup domain cache
	domainCacheMutex.Lock()
	for domain, result := range domainCache {
		if now.Sub(result.ValidatedAt) > domainCacheTTL {
			delete(domainCache, domain)
		}
	}
	domainCacheMutex.Unlock()
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Service:   "tm2-truemail-2.0",
		Version:   "2.0.0",
		Timestamp: time.Now().UTC(),
		Stats: HealthStats{
			CacheSize: len(mxCache) + len(domainCache),
			WorkerPoolSize: WORKER_POOL_SIZE,
			ConnectionPoolSize: CONNECTION_POOL_SIZE,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleSingleValidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	startTime := time.Now()
	response := validateEmailTM2(req.Email)
	response.ValidatedAt = time.Now().UTC()
	
	logger.Printf("✅ Single validation: %s -> %s (%.2fs)", 
		req.Email, response.Status, time.Since(startTime).Seconds())
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleBulkValidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BulkValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	startTime := time.Now()
	
	if !req.Quiet {
		logger.Printf("🚀 TM2 Bulk validation starting: %d emails", len(req.Emails))
	}
	
	// Process emails in parallel batches
	results := processBulkValidationTM2(req.Emails, req.Quiet)
	
	duration := time.Since(startTime)
	speed := float64(len(req.Emails)) / duration.Seconds()
	
	// Calculate stats
	stats := calculateBulkStats(results, duration)
	
	response := BulkValidationResponse{
		Results: results,
		Stats:   stats,
	}
	
	if !req.Quiet {
		logger.Printf("✅ TM2 Bulk validation completed: %d emails in %.2fs (%.1f emails/sec)", 
			len(req.Emails), duration.Seconds(), speed)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func processBulkValidationTM2(emails []string, quiet bool) []ValidationResponse {
	results := make([]ValidationResponse, len(emails))
	var wg sync.WaitGroup
	
	// Process in batches for optimal performance
	for i := 0; i < len(emails); i += BULK_BATCH_SIZE {
		end := i + BULK_BATCH_SIZE
		if end > len(emails) {
			end = len(emails)
		}
		
		batch := emails[i:end]
		
		// Process batch in parallel
		for j, email := range batch {
			wg.Add(1)
			go func(index int, emailAddr string) {
				defer wg.Done()
				
				// Acquire worker from pool
				workerPool <- struct{}{}
				defer func() { <-workerPool }()
				
				results[i+index] = validateEmailTM2(emailAddr)
			}(j, email)
		}
		
		wg.Wait()
	}
	
	return results
}

func validateEmailTM2(email string) ValidationResponse {
	
	response := ValidationResponse{
		Email:       email,
		ValidatedAt: time.Now().UTC(),
	}

	// Check rate limiting
	if !checkRateLimitTM2(email) {
		response.Status = "rate_limited"
		response.Score = 0
		response.Details = "Rate limit exceeded"
		response.ValidationMethod = "rate_limit"
		return response
	}

	// Perform progressive TM2 validation
	result := performProgressiveValidationTM2(email)

	// Determine status and score based on validation results
	if !result.IsValidFormat {
		response.Status = "invalid"
		response.Score = 0
		response.Details = "Invalid email format"
		response.ValidationMethod = "format_check"
	} else if result.IsDisposable {
		response.Status = "invalid"
		response.Score = 0
		response.Details = "Disposable email address detected"
		response.ValidationMethod = "disposable_check"
	} else if result.IsRoleBased {
		response.Status = "valid"
		response.Score = 70
		response.Details = "Role-based email address (lower deliverability)"
		response.ValidationMethod = "role_check"
	} else if result.IsValidSMTP && result.IsValidIMAP {
		response.Status = "valid"
		response.Score = 100
		response.Details = "Email validated with SMTP and IMAP checks"
		response.ValidationMethod = "smtp_imap"
	} else if result.IsValidSMTP {
		response.Status = "valid"
		response.Score = 90
		response.Details = "Email validated with SMTP check"
		response.ValidationMethod = "smtp_only"
	} else if result.HasMXRecords && result.IsValidDomain {
		response.Status = "unknown"
		response.Score = 60
		response.Details = "Domain and MX records valid, but SMTP validation failed"
		response.ValidationMethod = "mx_check"
	} else if result.IsValidDomain {
		response.Status = "unknown"
		response.Score = 40
		response.Details = "Domain valid but no MX records found"
		response.ValidationMethod = "dns_check"
	} else {
		response.Status = "invalid"
		response.Score = 0
		response.Details = "Invalid domain or no DNS records"
		response.ValidationMethod = "dns_check"
	}

	return response
}

func performProgressiveValidationTM2(email string) EmailValidationResult {
	result := EmailValidationResult{ValidationSteps: []string{}}
	startTime := time.Now()

	// 1. Format validation (instant)
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	result.IsValidFormat = emailRegex.MatchString(email)
	result.ValidationSteps = append(result.ValidationSteps, "Format validation")
	
	if !result.IsValidFormat {
		result.Details = "Invalid email format"
		result.Duration = time.Since(startTime)
		return result
	}

	// 2. Extract domain and local part
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		result.Details = "Invalid email structure"
		result.Duration = time.Since(startTime)
		return result
	}
	localPart := parts[0]
	domain := parts[1]

	// 3. Disposable email check (instant)
	result.IsDisposable = checkDisposableEmailTM2(domain)
	result.ValidationSteps = append(result.ValidationSteps, "Disposable email check")
	if result.IsDisposable {
		result.Details = "Disposable email domain detected"
		result.Duration = time.Since(startTime)
		return result
	}

	// 4. Role-based email check (instant)
	result.IsRoleBased = checkRoleBasedEmailTM2(localPart)
	result.ValidationSteps = append(result.ValidationSteps, "Role-based email check")

	// 5. Domain validation with DNS lookup (fast, cached)
	result.IsValidDomain = validateDomainWithDNSTM2(domain)
	result.ValidationSteps = append(result.ValidationSteps, "DNS domain validation")
	if !result.IsValidDomain {
		result.Details = "Domain does not exist or is invalid"
		result.Duration = time.Since(startTime)
		return result
	}

	// 6. MX records check (fast, cached)
	result.HasMXRecords = checkMXRecordsTM2(domain)
	result.ValidationSteps = append(result.ValidationSteps, "MX records check")
	if !result.HasMXRecords {
		result.Details = "No MX records found for domain"
		result.Duration = time.Since(startTime)
		return result
	}

	// 7. SMTP validation (moderate, with connection pooling)
	result.IsValidSMTP = validateEmailWithSMTPTM2(email)
	result.ValidationSteps = append(result.ValidationSteps, "SMTP validation")

	// 8. IMAP validation (only for major providers)
	if isMajorProvider(domain) {
		result.IsValidIMAP = validateEmailWithIMAPTM2(email)
		result.ValidationSteps = append(result.ValidationSteps, "IMAP validation")
	} else {
		result.IsValidIMAP = false
		result.ValidationSteps = append(result.ValidationSteps, "IMAP skipped (not major provider)")
	}

	result.Details = "Email validation completed successfully"
	result.Duration = time.Since(startTime)
	return result
}

func checkRateLimitTM2(email string) bool {
	rateMutex.Lock()
	defer rateMutex.Unlock()
	
	now := time.Now()
	if lastTime, exists := rateLimiter[email]; exists {
		if now.Sub(lastTime) < 1*time.Second { // Reduced to 1 second
			return false
		}
	}
	rateLimiter[email] = now
	return true
}

func checkDisposableEmailTM2(domain string) bool {
	return disposableDomains[strings.ToLower(domain)]
}

func checkRoleBasedEmailTM2(localPart string) bool {
	lowerLocalPart := strings.ToLower(localPart)
	for _, pattern := range rolePatterns {
		if strings.HasPrefix(lowerLocalPart, pattern) {
			return true
		}
	}
	return false
}

func isMajorProvider(domain string) bool {
	return majorProviders[strings.ToLower(domain)]
}

func validateDomainWithDNSTM2(domain string) bool {
	// Check cache first
	domainCacheMutex.RLock()
	if result, exists := domainCache[domain]; exists {
		domainCacheMutex.RUnlock()
		return result.IsValid
	}
	domainCacheMutex.RUnlock()
	
	// Get DNS client from pool
	dnsClient := <-dnsClientPool
	defer func() { dnsClientPool <- dnsClient }()
	
	m := new(dns.Msg)
	m.SetQuestion(domain+".", dns.TypeA)
	
	resp, _, err := dnsClient.Exchange(m, "8.8.8.8:53")
	if err != nil {
		return false
	}
	
	isValid := len(resp.Answer) > 0
	
	// Cache result
	domainCacheMutex.Lock()
	domainCache[domain] = DomainValidationResult{
		IsValid: isValid,
		ValidatedAt: time.Now(),
	}
	domainCacheMutex.Unlock()
	
	return isValid
}

func checkMXRecordsTM2(domain string) bool {
	// Check cache first
	mxCacheMutex.RLock()
	if mxServers, exists := mxCache[domain]; exists {
		mxCacheMutex.RUnlock()
		return len(mxServers) > 0
	}
	mxCacheMutex.RUnlock()
	
	// Get DNS client from pool
	dnsClient := <-dnsClientPool
	defer func() { dnsClientPool <- dnsClient }()
	
	m := new(dns.Msg)
	m.SetQuestion(domain+".", dns.TypeMX)
	
	resp, _, err := dnsClient.Exchange(m, "8.8.8.8:53")
	if err != nil {
		return false
	}
	
	var mxServers []string
	for _, answer := range resp.Answer {
		if mx, ok := answer.(*dns.MX); ok {
			mxServers = append(mxServers, mx.Mx)
		}
	}
	
	// Cache result
	mxCacheMutex.Lock()
	mxCache[domain] = mxServers
	mxCacheMutex.Unlock()
	
	return len(mxServers) > 0
}

func validateEmailWithSMTPTM2(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]

	// Get MX records (cached)
	mxCacheMutex.RLock()
	mxServers, exists := mxCache[domain]
	mxCacheMutex.RUnlock()
	
	if !exists {
		// Get MX records if not cached
		dnsClient := <-dnsClientPool
		defer func() { dnsClientPool <- dnsClient }()
		
		m := new(dns.Msg)
		m.SetQuestion(domain+".", dns.TypeMX)
		
		resp, _, err := dnsClient.Exchange(m, "8.8.8.8:53")
		if err != nil || len(resp.Answer) == 0 {
			return false
		}
		
		for _, answer := range resp.Answer {
			if mx, ok := answer.(*dns.MX); ok {
				mxServers = append(mxServers, mx.Mx)
			}
		}
		
		// Cache MX servers
		mxCacheMutex.Lock()
		mxCache[domain] = mxServers
		mxCacheMutex.Unlock()
	}
	
	if len(mxServers) == 0 {
		return false
	}

	// Try first MX server
	mxHost := mxServers[0]

	// Establish SMTP connection with optimized timeout
	dialer := &net.Dialer{Timeout: SMTP_TIMEOUT}
	conn, err := dialer.Dial("tcp", mxHost+":25")
	if err != nil {
		return false
	}
	defer conn.Close()

	// Set connection deadline
	conn.SetDeadline(time.Now().Add(SMTP_TIMEOUT + 1*time.Second))

	// Create SMTP client
	client, err := smtp.NewClient(conn, mxHost)
	if err != nil {
		return false
	}
	defer client.Close()

	// Perform SMTP handshake
	if err := client.Hello("tm2-validation.example.com"); err != nil {
		return false
	}

	// Set sender
	if err := client.Mail("validation@tm2.example.com"); err != nil {
		return false
	}

	// Check recipient
	if err := client.Rcpt(email); err != nil {
		return false
	}

	return true
}

func validateEmailWithIMAPTM2(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]

	// Only validate major providers
	if !isMajorProvider(domain) {
		return false
	}

	// Common IMAP servers for major providers
	imapServers := map[string]string{
		"gmail.com": "imap.gmail.com:993",
		"yahoo.com": "imap.mail.yahoo.com:993",
		"outlook.com": "outlook.office365.com:993",
		"hotmail.com": "outlook.office365.com:993",
		"live.com": "outlook.office365.com:993",
		"icloud.com": "imap.mail.me.com:993",
		"aol.com": "imap.aol.com:993",
		"protonmail.com": "127.0.0.1:1143", // ProtonMail doesn't allow external IMAP
	}

	imapServer, exists := imapServers[domain]
	if !exists {
		return false
	}

	// Establish IMAP connection with optimized timeout
	
	c, err := client.DialTLS(imapServer, nil)
	if err != nil {
		return false
	}
	defer c.Logout()

	// Set timeout
	c.Timeout = IMAP_TIMEOUT

	// Try anonymous login
	if err := c.Login("anonymous", "anonymous"); err != nil {
		return false
	}

	// Select INBOX
	_, err = c.Select("INBOX", false)
	if err != nil {
		return false
	}

	return true
}

func calculateBulkStats(results []ValidationResponse, duration time.Duration) BulkStats {
	stats := BulkStats{
		Total: len(results),
		Duration: duration.Seconds(),
	}
	
	for _, result := range results {
		switch result.Status {
		case "valid":
			stats.Valid++
		case "invalid":
			stats.Invalid++
		case "unknown":
			stats.Unknown++
		case "rate_limited":
			stats.RateLimited++
		}
	}
	
	if duration.Seconds() > 0 {
		stats.Speed = float64(len(results)) / duration.Seconds()
	}
	
	return stats
} 