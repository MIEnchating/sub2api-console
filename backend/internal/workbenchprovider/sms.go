package workbenchprovider

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
)

type SMSConfig struct {
	Provider      string `json:"-"`
	APIKey        string `json:"-"`
	ServiceID     string `json:"-"`
	Service       string `json:"-"`
	Country       string `json:"-"`
	MaxPrice      string `json:"-"`
	CustomEntries string `json:"-"`
}

type SMSNumber struct {
	RequestID string `json:"-"`
	Phone     string `json:"-"`
}

type SMSMessage struct {
	Pending bool   `json:"pending"`
	Code    string `json:"-"`
}

type SMSOption struct {
	Country string `json:"country"`
	Title   string `json:"title"`
	ISO     string `json:"iso"`
	Prefix  string `json:"prefix"`
	Price   string `json:"price"`
	Count   int64  `json:"count"`
}

type SMSError struct {
	Code      string
	Message   string
	Terminal  bool
	Uncertain bool
}

func (e *SMSError) Error() string { return e.Message }

type smsAcquisition struct {
	number SMSNumber
	err    error
}

type smsActivation struct {
	operation  string
	number     SMSNumber
	closed     bool
	readOnly   bool
	actions    map[string]error
	custom     *smsCustomEntry
	seenKeys   map[string]bool
	seenCodes  map[string]bool
	acquiredAt time.Time
}

type SMS struct {
	journal      SMSJournal
	mu           sync.Mutex
	config       SMSConfig
	http         *HTTP
	ownsHTTP     bool
	acquisitions map[string]smsAcquisition
	activations  map[string]*smsActivation
	entries      []smsCustomEntry
	pool         *SMSPool
	closed       bool
}

var smsAPIKey = regexp.MustCompile(`^[A-Za-z0-9._-]{8,256}$`)
var smsServiceID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,80}$`)
var smsService = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
var smsCountry = regexp.MustCompile(`^[0-9]{1,5}$`)
var smsRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
var smsPhone = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
var smsPrice = regexp.MustCompile(`^[0-9]+(?:\.[0-9]{1,64})?$`)

func NewSMS(config SMSConfig, client *HTTP, pools ...*SMSPool) (*SMS, error) {
	config.Provider = strings.ToLower(strings.TrimSpace(config.Provider))
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.ServiceID = strings.TrimSpace(config.ServiceID)
	config.Service = strings.ToLower(strings.TrimSpace(config.Service))
	config.Country = strings.TrimSpace(config.Country)
	config.MaxPrice = strings.TrimSpace(config.MaxPrice)
	ownsHTTP := client == nil
	if ownsHTTP {
		client = NewHTTP(nil)
	}
	service := &SMS{config: config, http: client, ownsHTTP: ownsHTTP, acquisitions: make(map[string]smsAcquisition), activations: make(map[string]*smsActivation)}
	if len(pools) > 1 {
		return nil, smsFailure("sms_pool_invalid", "接码号码池配置无效", true)
	}
	service.pool = NewSMSPool()
	if len(pools) == 1 && pools[0] != nil {
		service.pool = pools[0]
	}
	switch config.Provider {
	case "smsbower":
		if config.Service == "" {
			service.config.Service = "dr"
		}
		if !smsAPIKey.MatchString(config.APIKey) || !smsService.MatchString(service.config.Service) || (config.Country != "" && !smsCountry.MatchString(config.Country)) {
			return nil, smsFailure("sms_config_invalid", "SMSBower 配置无效，请检查 API Key、服务代码和国家 ID", true)
		}
		if config.MaxPrice != "" && (!smsPrice.MatchString(config.MaxPrice) || !positiveSMSPrice(config.MaxPrice)) {
			return nil, smsFailure("sms_price_invalid", "最高价格无效，请重新查询供应商国家价格", true)
		}
	case "luban":
		if !smsAPIKey.MatchString(config.APIKey) || !smsServiceID.MatchString(config.ServiceID) {
			return nil, smsFailure("sms_config_invalid", "LubanSMS 配置无效，请检查 API Key 和供应商编号", true)
		}
	case "custom":
		entries, err := parseSMSEntries(config.CustomEntries)
		if err != nil {
			return nil, err
		}
		service.entries = entries
		service.config.CustomEntries = ""
	default:
		return nil, smsFailure("sms_provider_invalid", "请选择 SMSBower、LubanSMS 或自定义接码服务", true)
	}
	return service, nil
}

func (s *SMS) Options(ctx context.Context) ([]SMSOption, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.active(ctx); err != nil {
		return nil, err
	}
	if s.config.Provider != "smsbower" {
		return []SMSOption{}, nil
	}
	return s.bowerOptions(ctx)
}

// Acquire is one-shot per caller-generated operation ID, including uncertain
// failures. A new authorization attempt must use a distinct operation ID.
func (s *SMS) Acquire(ctx context.Context, operationID string) (SMSNumber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.active(ctx); err != nil {
		return SMSNumber{}, err
	}
	if !smsRequestID.MatchString(operationID) {
		return SMSNumber{}, smsFailure("sms_operation_invalid", "接码请求 ID 无效，请重新创建授权任务", true)
	}
	if previous, exists := s.acquisitions[operationID]; exists {
		return previous.number, previous.err
	}
	if len(s.acquisitions) >= 500 {
		return SMSNumber{}, smsFailure("sms_batch_limit", "本批号码申请已达到 500 次限制，请创建新的授权批次", true)
	}
	if err := s.journalWrite(ctx, operationID, "acquire", "submitted", SMSNumber{}); err != nil {
		s.acquisitions[operationID] = smsAcquisition{err: err}
		return SMSNumber{}, err
	}
	var number SMSNumber
	var activation *smsActivation
	var err error
	switch s.config.Provider {
	case "smsbower":
		number, err = s.bowerAcquire(ctx)
	case "luban":
		number, err = s.lubanAcquire(ctx)
	case "custom":
		activation, err = s.customAcquire(ctx)
		if activation != nil {
			number = activation.number
		}
	}
	if err == nil {
		if _, exists := s.activations[number.RequestID]; exists {
			err = smsFailure("sms_duplicate_activation", "供应商返回重复订单 ID，请核对供应商订单后重新授权", true)
			number = SMSNumber{}
		} else {
			if activation == nil {
				activation = &smsActivation{number: number, actions: make(map[string]error)}
			}
			s.activations[number.RequestID] = activation
		}
	}
	if err != nil && number.RequestID != "" {
		if _, exists := s.activations[number.RequestID]; exists {
			number = SMSNumber{}
		} else {
			// A valid order ID remains available for explicit cancellation even
			// when the provider returned an unusable phone number.
			s.activations[number.RequestID] = &smsActivation{number: number, actions: make(map[string]error)}
		}
	}
	if activation := s.activations[number.RequestID]; activation != nil {
		activation.operation = operationID
	}
	state := "confirmed"
	if err != nil {
		state = "uncertain"
	}
	if journalErr := s.journalWrite(ctx, operationID, "acquire", state, number); journalErr != nil {
		err = journalErr
	}
	s.acquisitions[operationID] = smsAcquisition{number: number, err: err}
	return number, err
}

func (s *SMS) Poll(ctx context.Context, requestID string) (SMSMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	activation, err := s.activation(ctx, requestID)
	if err != nil {
		return SMSMessage{}, err
	}
	switch s.config.Provider {
	case "smsbower":
		return s.bowerPoll(ctx, requestID)
	case "luban":
		return s.lubanPoll(ctx, requestID)
	default:
		return s.customPoll(ctx, activation)
	}
}

func (s *SMS) MarkReady(ctx context.Context, requestID string) error {
	return s.setStatus(ctx, requestID, "ready")
}

func (s *SMS) Complete(ctx context.Context, requestID string) error {
	return s.setStatus(ctx, requestID, "complete")
}

func (s *SMS) Release(ctx context.Context, requestID string) error {
	return s.setStatus(ctx, requestID, "release")
}

func (s *SMS) setStatus(ctx context.Context, requestID, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.active(ctx); err != nil {
		return err
	}
	activation := s.activations[requestID]
	if activation == nil {
		return smsFailure("sms_activation_missing", "接码订单不属于当前授权会话", true)
	}
	if activation.readOnly {
		return smsFailure("sms_activation_read_only", "恢复的短信订单仅允许核对状态，请在供应商处理订单", true)
	}
	if previous, exists := activation.actions[action]; exists {
		return previous
	}
	if activation.closed {
		return smsFailure("sms_activation_closed", "接码订单已结束，请重新授权", true)
	}
	if err := s.journalWrite(ctx, activation.operation, action, "submitted", activation.number); err != nil {
		activation.actions[action] = err
		return err
	}
	var err error
	switch s.config.Provider {
	case "smsbower":
		err = s.bowerStatus(ctx, requestID, action)
	case "luban":
		if action == "release" {
			err = s.lubanRelease(ctx, requestID)
		}
	}
	state := "confirmed"
	if err != nil {
		state = "uncertain"
	}
	if journalErr := s.journalWrite(ctx, activation.operation, action, state, activation.number); journalErr != nil {
		err = journalErr
	}
	activation.actions[action] = err
	if action != "ready" {
		activation.closed = true
		activation.seenKeys, activation.seenCodes = nil, nil
	}
	return err
}

func (s *SMS) active(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.closed {
		return smsFailure("sms_closed", "接码会话已关闭，请重新授权", true)
	}
	return nil
}

func (s *SMS) activation(ctx context.Context, requestID string) (*smsActivation, error) {
	if err := s.active(ctx); err != nil {
		return nil, err
	}
	activation := s.activations[requestID]
	if activation == nil || !smsRequestID.MatchString(requestID) {
		return nil, smsFailure("sms_activation_missing", "接码订单不属于当前授权会话", true)
	}
	if activation.closed {
		return nil, smsFailure("sms_activation_closed", "接码订单已结束，请重新授权", true)
	}
	return activation, nil
}

// Close drops credentials and received codes. Completing or releasing a paid
// order is a separate, explicit domain action.
func (s *SMS) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.config = SMSConfig{}
	s.entries, s.acquisitions, s.activations = nil, nil, nil
	if s.ownsHTTP {
		s.http.Close()
	}
}

func smsFailure(code, message string, terminal bool) *SMSError {
	return &SMSError{Code: code, Message: message, Terminal: terminal}
}

func uncertainSMS(err error) error {
	var provider *SMSError
	if errors.As(err, &provider) {
		return err
	}
	return &SMSError{Code: "sms_commit_unknown", Message: "供应商提交结果尚未确定，请先核对订单；本次请求不会自动重发", Uncertain: true}
}

func normalizeSMSPhone(raw string) (string, error) {
	phone := strings.TrimSpace(raw)
	phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", "\t", "", "\r", "", "\n", "").Replace(phone)
	if strings.HasPrefix(phone, "00") {
		phone = "+" + phone[2:]
	} else if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}
	if !smsPhone.MatchString(phone) {
		return "", smsFailure("sms_phone_invalid", "供应商返回的号码不是有效的国际电话号码，请核对订单", true)
	}
	return phone, nil
}
