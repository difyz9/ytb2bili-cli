package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

// ==================== TOML 配置 ====================

type Config struct {
	WechatPay WechatPayConfig `toml:"WechatPay"`
	SkillHub  SkillHubConfig  `toml:"SkillHub"`
	AIPreoder AIPreorder      `toml:"AIPreorder"`
}

type WechatPayConfig struct {
	AppId           string `toml:"AppId"`
	MchId           string `toml:"MchId"`
	SerialNo        string `toml:"SerialNo"`
	PrivateKey      string `toml:"PrivateKey"`
	ApiV3Key        string `toml:"ApiV3Key"`
	NotifyURL       string `toml:"NotifyURL"`
	RefundNotifyURL string `toml:"RefundNotifyURL"`
}

type SkillHubConfig struct {
	DeveloperId    string `toml:"DeveloperId"`
	PubKeyId       string `toml:"PubKeyId"`
	PrivateKeyFile string `toml:"PrivateKeyFile"`
	SkillId        string `toml:"SkillId"`
	SkillVersion   string `toml:"SkillVersion"`
}

type AIPreorder struct {
	URL string `toml:"URL"`
}

func loadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	return &cfg, nil
}

// ==================== 微信支付 SDK 客户端 ====================

var wxPayClient *core.Client
var wxNotifyHandler *notify.Handler

var (
	cfg          *Config
	skillhubPriv *rsa.PrivateKey // 缓存的 SkillHub 私钥
)

func initPayClient() {
	mchPrivateKey, err := utils.LoadPrivateKeyWithPath(cfg.WechatPay.PrivateKey)
	if err != nil {
		log.Fatalf("加载微信支付商户私钥失败: %v", err)
	}

	ctx := context.Background()
	opts := []core.ClientOption{
		option.WithWechatPayAutoAuthCipher(cfg.WechatPay.MchId, cfg.WechatPay.SerialNo, mchPrivateKey, cfg.WechatPay.ApiV3Key),
	}
	client, err := core.NewClient(ctx, opts...)
	if err != nil {
		log.Fatalf("初始化微信支付客户端失败: %v", err)
	}
	wxPayClient = client
	log.Println("微信支付 SDK 客户端初始化成功")

	certDownloader := downloader.NewCertificateDownloaderMgr(ctx)
	if err := certDownloader.RegisterDownloaderWithPrivateKey(
		ctx, mchPrivateKey, cfg.WechatPay.SerialNo, cfg.WechatPay.MchId, cfg.WechatPay.ApiV3Key,
	); err != nil {
		log.Fatalf("注册证书下载器失败: %v", err)
	}
	certVisitor := certDownloader.GetCertificateVisitor(cfg.WechatPay.MchId)
	certVerifier := verifiers.NewSHA256WithRSAVerifier(certVisitor)
	wxNotifyHandler, err = notify.NewRSANotifyHandler(cfg.WechatPay.ApiV3Key, certVerifier)
	if err != nil {
		log.Fatalf("初始化回调通知处理器失败: %v", err)
	}
	log.Println("微信支付回调通知处理器初始化成功")
}

func loadSkillHubKey() {
	keyBytes, err := os.ReadFile(cfg.SkillHub.PrivateKeyFile)
	if err != nil {
		log.Fatalf("读取 SkillHub 私钥文件失败: %v", err)
	}
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		log.Fatalf("无法解析 SkillHub 私钥 PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			log.Fatalf("解析 SkillHub 私钥失败: %v", err)
		}
	}
	var ok bool
	skillhubPriv, ok = key.(*rsa.PrivateKey)
	if !ok {
		log.Fatalf("SkillHub 私钥不是 RSA 类型")
	}
}

// ==================== 数据结构 ====================

type OrderRecord struct {
	OutTradeNo   string    `json:"out_trade_no"`
	PaymentCode  string    `json:"payment_code"`
	Amount       int       `json:"amount"`
	Description  string    `json:"description"`
	Query        string    `json:"query"`
	Status       string    `json:"status"`
	Result       string    `json:"result"`
	RefundNo     string    `json:"refund_no"`
	RefundReason string    `json:"refund_reason"`
	CreatedAt    time.Time `json:"created_at"`
}

type OrderStore struct {
	mu     sync.RWMutex
	orders map[string]*OrderRecord
}

func NewOrderStore() *OrderStore {
	return &OrderStore{orders: make(map[string]*OrderRecord)}
}

func (s *OrderStore) Save(order *OrderRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[order.OutTradeNo] = order
}

func (s *OrderStore) Get(outTradeNo string) *OrderRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orders[outTradeNo]
}

type ResourceRequest struct {
	Query string `json:"query"`
}

type L2PaymentRequired struct {
	SkillInfo *SkillInfo `json:"skill_info"`
	PayType   string     `json:"pay_type"`
	PayMode   string     `json:"pay_mode"`
	PayItems  []PayItem  `json:"pay_items"`
	ExpiresAt string     `json:"expires_at"`
}

type SkillInfo struct {
	SkillID      string `json:"skill_id"`
	SkillVersion string `json:"skill_version"`
}

type PayItem struct {
	ProductID string   `json:"product_id"`
	PayData   *PayData `json:"pay_data"`
}

type PayData struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type L1PreorderRequest struct {
	SignatureType     string `json:"signature_type"`
	DeveloperPlatform string `json:"developer_platform"`
	DeveloperID       string `json:"developer_id"`
	PubKeyID          string `json:"pub_key_id"`
	NonceStr          string `json:"nonce_str"`
	Timestamp         string `json:"timestamp"`
	Signature         string `json:"signature"`
	PaymentRequired   string `json:"payment_required"`
}

type PreorderResponse struct {
	PaymentCode string `json:"payment_code"`
}

// ==================== 主程序 ====================

var store = NewOrderStore()

func main() {
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "config.toml"
	}
	var err error
	cfg, err = loadConfig(configPath)
	if err != nil {
		log.Fatal(err)
	}

	initPayClient()
	loadSkillHubKey()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/api/resource", handleResource)
	http.HandleFunc("/api/pay/notify", handlePayNotify)
	http.HandleFunc("/api/refund/notify", handleRefundNotify)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Printf("商户 Demo 启动，监听端口: %s", port)
	log.Printf("配置: %s", configPath)
	log.Printf("接口: POST http://localhost:%s/api/resource", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "仅支持 POST"})
		return
	}
	var req ResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	outTradeNo := r.Header.Get("X-Out-Trade-No")
	if outTradeNo != "" {
		verifyAndFulfill(w, outTradeNo, req.Query)
		return
	}
	createPaymentOrder(w, req)
}

func createPaymentOrder(w http.ResponseWriter, req ResourceRequest) {
	outTradeNo := generateOutTradeNo()
	amount := int64(1)
	description := fmt.Sprintf("AI付费查询: %s", truncate(req.Query, 50))

	codeURL, err := callNativeOrder(outTradeNo, description, amount)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "下单失败: " + err.Error()})
		return
	}

	paymentCode, err := callAIPreorder(codeURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "AI预下单失败: " + err.Error()})
		return
	}

	order := &OrderRecord{
		OutTradeNo:  outTradeNo,
		PaymentCode: paymentCode,
		Amount:      int(amount),
		Description: description,
		Query:       req.Query,
		Status:      "INIT",
		CreatedAt:   time.Now(),
	}
	store.Save(order)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("WeixinPay-Required", paymentCode)
	w.Header().Set("X-Out-Trade-No", outTradeNo)
	w.WriteHeader(http.StatusPaymentRequired)

	json.NewEncoder(w).Encode(map[string]any{
		"code":    "PAYMENT_REQUIRED",
		"message": "需要支付后才能获取内容",
		"WeixinPay": map[string]any{
			"WeixinPay-Required": paymentCode,
		},
		"out_trade_no": outTradeNo,
		"amount":       fmt.Sprintf("%.2f", float64(amount)/100),
		"currency":     "CNY",
		"description":  description,
	})
}

func verifyAndFulfill(w http.ResponseWriter, outTradeNo string, query string) {
	order := store.Get(outTradeNo)
	if order == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "订单不存在: " + outTradeNo})
		return
	}
	if order.Status == "FULFILLED" {
		writeJSON(w, http.StatusOK, map[string]any{
			"code": "SUCCESS", "content": order.Result,
			"out_trade_no": outTradeNo, "already_fulfilled": true,
		})
		return
	}

	tradeState, _, err := callQueryOrder(outTradeNo)
	if err != nil || tradeState != "SUCCESS" {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{
			"code": "NOT_PAID", "message": "订单尚未支付完成",
			"out_trade_no": outTradeNo,
		})
		return
	}

	bizQuery := query
	if bizQuery == "" {
		bizQuery = order.Query
	}
	result, err := executeBusiness(bizQuery)
	if err != nil {
		refundErr := callRefund(outTradeNo, int64(order.Amount), "服务异常: "+err.Error())
		if refundErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"code": "REFUND_FAILED", "message": "退款失败"})
			return
		}
		order.Status = "REFUNDED"
		store.Save(order)
		writeJSON(w, http.StatusOK, map[string]any{"code": "REFUNDED", "message": "已发起全额退款", "out_trade_no": outTradeNo})
		return
	}

	order.Status = "FULFILLED"
	order.Result = result
	store.Save(order)

	writeJSON(w, http.StatusOK, map[string]any{
		"code": "SUCCESS", "content": result,
		"out_trade_no": outTradeNo, "already_fulfilled": false,
	})
}

func executeBusiness(query string) (string, error) {
	if query == "trigger_refund" {
		return "", fmt.Errorf("下游服务不可用")
	}
	return generatePaidContent(query), nil
}

func callNativeOrder(outTradeNo, description string, amount int64) (string, error) {
	svc := native.NativeApiService{Client: wxPayClient}
	timeExpire := time.Now().Add(14*time.Minute + 30*time.Second)
	resp, _, err := svc.Prepay(context.Background(), native.PrepayRequest{
		Appid: core.String(cfg.WechatPay.AppId), Mchid: core.String(cfg.WechatPay.MchId),
		Description: core.String(description), OutTradeNo: core.String(outTradeNo),
		NotifyUrl: core.String(cfg.WechatPay.NotifyURL), TimeExpire: core.Time(timeExpire),
		Amount: &native.Amount{Total: core.Int64(amount), Currency: core.String("CNY")},
	})
	if err != nil {
		return "", fmt.Errorf("Native下单失败: %w", err)
	}
	return *resp.CodeUrl, nil
}

func callQueryOrder(outTradeNo string) (string, string, error) {
	svc := native.NativeApiService{Client: wxPayClient}
	resp, _, err := svc.QueryOrderByOutTradeNo(context.Background(), native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(outTradeNo), Mchid: core.String(cfg.WechatPay.MchId),
	})
	if err != nil {
		return "", "", fmt.Errorf("查单失败: %w", err)
	}
	tradeState := *resp.TradeState
	txnID := ""
	if resp.TransactionId != nil {
		txnID = *resp.TransactionId
	}
	return tradeState, txnID, nil
}

func callRefund(outTradeNo string, amount int64, reason string) error {
	svc := refunddomestic.RefundsApiService{Client: wxPayClient}
	outRefundNo := "RF_" + outTradeNo[6:]
	_, _, err := svc.Create(context.Background(), refunddomestic.CreateRequest{
		OutTradeNo: core.String(outTradeNo), OutRefundNo: core.String(outRefundNo),
		Reason: core.String(truncate(reason, 80)), NotifyUrl: core.String(cfg.WechatPay.RefundNotifyURL),
		Amount: &refunddomestic.AmountReq{
			Refund: core.Int64(amount), Total: core.Int64(amount), Currency: core.String("CNY"),
		},
	})
	return err
}

func handlePayNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	content := make(map[string]interface{})
	_, err := wxNotifyHandler.ParseNotifyRequest(r.Context(), r, content)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "FAIL", "message": err.Error()})
		return
	}
	outTradeNo, _ := content["out_trade_no"].(string)
	tradeState, _ := content["trade_state"].(string)
	if tradeState == "SUCCESS" && outTradeNo != "" {
		if order := store.Get(outTradeNo); order != nil && order.Status == "INIT" {
			order.Status = "PAID"
			store.Save(order)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": "SUCCESS"})
}

func handleRefundNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	refund := new(refunddomestic.Refund)
	_, err := wxNotifyHandler.ParseNotifyRequest(r.Context(), r, refund)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "FAIL", "message": err.Error()})
		return
	}
	if refund.OutTradeNo != nil {
		if order := store.Get(*refund.OutTradeNo); order != nil {
			if refund.Status != nil && *refund.Status == refunddomestic.STATUS_SUCCESS {
				order.Status = "REFUNDED"
				store.Save(order)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": "SUCCESS"})
}

func callAIPreorder(codeURL string) (string, error) {
	l2 := L2PaymentRequired{
		SkillInfo: &SkillInfo{SkillID: cfg.SkillHub.SkillId, SkillVersion: cfg.SkillHub.SkillVersion},
		PayType:   "SKILL_PAY",
		PayMode:   "AUTH_AND_PAY",
		PayItems: []PayItem{{
			ProductID: "SP" + time.Now().Format("060102") + "A001",
			PayData:   &PayData{Type: "code_url", Value: codeURL},
		}},
		ExpiresAt: fmt.Sprintf("%d", time.Now().Add(15*time.Minute).Unix()),
	}
	l2Bytes, _ := json.Marshal(l2)
	paymentRequired := base64.StdEncoding.EncodeToString(l2Bytes)

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonceStr := generateNonceStr()
	signString := fmt.Sprintf("POST\n/palmpayminiapp/clawagentpay/preorder\n%s\n%s\n%s\n",
		timestamp, nonceStr, paymentRequired)

	hashed := sha256.Sum256([]byte(signString))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, skillhubPriv, crypto.SHA256, hashed[:])
	signature := base64.StdEncoding.EncodeToString(sig)

	l1 := L1PreorderRequest{
		SignatureType: "SKILLHUB-SHA256-RSA2048", DeveloperPlatform: "SKILLHUB",
		DeveloperID: cfg.SkillHub.DeveloperId, PubKeyID: cfg.SkillHub.PubKeyId,
		NonceStr: nonceStr, Timestamp: timestamp,
		Signature: signature, PaymentRequired: paymentRequired,
	}
	l1Bytes, _ := json.Marshal(l1)

	req, _ := http.NewRequest("POST", cfg.AIPreoder.URL, bytes.NewReader(l1Bytes))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI预下单失败 HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result PreorderResponse
	json.Unmarshal(body, &result)
	return result.PaymentCode, nil
}

func generateOutTradeNo() string {
	ts := time.Now().Format("20060102150405")
	b := make([]byte, 6)
	rand.Read(b)
	return fmt.Sprintf("WX402_%s%s", ts, hex.EncodeToString(b))
}

func generateNonceStr() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generatePaidContent(query string) string {
	return fmt.Sprintf("【付费内容】针对「%s」的分析结果\n生成时间: %s",
		query, time.Now().Format("2006-01-02 15:04:05"))
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
