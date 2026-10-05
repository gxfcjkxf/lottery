package identity

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
	"strings"
	"time"
)

var ErrSession = errors.New("session unavailable")
var ErrBusy = errors.New("password hashing busy")

type Store struct {
	DB        *pgxpool.Pool
	TTL       time.Duration
	dummyHash string
	slots     chan struct{}
}

func New(db *pgxpool.Pool) (*Store, error) {
	dummy, err := authcrypto.HashPassword("not-a-real-password")
	if err != nil {
		return nil, err
	}
	return &Store{DB: db, TTL: 24 * time.Hour, dummyHash: dummy, slots: make(chan struct{}, 4)}, nil
}

type User struct {
	ID         string `json:"id"`
	Username   string `json:"username,omitempty"`
	Phone      string `json:"phone,omitempty"`
	TelegramID string `json:"telegram_user_id,omitempty"`
	Status     string `json:"status"`
}
type Member struct {
	ID          string    `json:"id"`
	BrandID     string    `json:"brand_id"`
	Status      string    `json:"status"`
	DisplayName string    `json:"display_name"`
	JoinedAt    time.Time `json:"joined_at"`
}
type View struct {
	User   User   `json:"user"`
	Member Member `json:"member"`
}
type Session struct {
	View
	ID string `json:"-"`
}
type Authentication struct {
	View
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}
type RegisterInput struct {
	CaptchaID     string `json:"captcha_id,omitempty"`
	CaptchaAnswer string `json:"captcha_answer,omitempty"`
	Username      string `json:"username"`
	Phone         string `json:"phone"`
	Password      string `json:"password"`
	Privacy       string `json:"privacy_policy_version"`
	Terms         string `json:"service_terms_version"`
}
type LoginInput struct {
	CaptchaID     string `json:"captcha_id,omitempty"`
	CaptchaAnswer string `json:"captcha_answer,omitempty"`
	Identifier    string `json:"identifier"`
	Password      string `json:"password"`
	Privacy       string `json:"privacy_policy_version,omitempty"`
	Terms         string `json:"service_terms_version,omitempty"`
}
type ProfileInput struct {
	Username string `json:"username,omitempty"`
	Phone    string `json:"phone,omitempty"`
}
type Metadata struct{ RequestID, IP, Domain string }
type Settings struct {
	CaptchaEnabled   bool   `json:"captcha_enabled"`
	TelegramEnabled  bool   `json:"telegram_enabled"`
	TelegramClientID string `json:"telegram_client_id,omitempty"`
	Privacy          string `json:"privacy_policy_version"`
	Terms            string `json:"service_terms_version"`
}

var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,31}$`)
var phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

func normalizeUsername(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if !usernamePattern.MatchString(v) {
		return "", errors.New("invalid username")
	}
	return v, nil
}
func normalizePhone(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !phonePattern.MatchString(v) {
		return "", errors.New("invalid phone")
	}
	return v, nil
}
func unique(err error) bool { var p *pgconn.PgError; return errors.As(err, &p) && p.Code == "23505" }
func tokenHash(token string) string {
	h := authcrypto.DigestSessionToken(token)
	return hex.EncodeToString(h[:])
}
func (s *Store) PasswordHash(ctx context.Context, password string) (string, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
		return authcrypto.HashPassword(password)
	case <-ctx.Done():
		return "", ctx.Err()
	default:
		return "", ErrBusy
	}
}
func (s *Store) verify(ctx context.Context, password, hash string) (bool, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
		return authcrypto.VerifyPassword(password, hash)
	case <-ctx.Done():
		return false, ctx.Err()
	default:
		return false, ErrBusy
	}
}
func (s *Store) Settings(ctx context.Context, brand string) (Settings, error) {
	var raw []byte
	var cfg Settings
	err := s.DB.QueryRow(ctx, "SELECT auth_config FROM brands WHERE id=$1", brand).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &cfg)
	}
	return cfg, err
}
func (s *Store) ReadAuthSettings(ctx context.Context, brand string) (AuthSettingsRecord, error) {
	var raw []byte
	var record AuthSettingsRecord
	err := s.DB.QueryRow(ctx, "SELECT config_version,auth_config FROM brands WHERE id=$1", brand).Scan(&record.Version, &raw)
	if err == nil {
		var cfg Settings
		err = json.Unmarshal(raw, &cfg)
		record.CaptchaEnabled = cfg.CaptchaEnabled
		record.TelegramEnabled = cfg.TelegramEnabled
		record.TelegramClientID = cfg.TelegramClientID
		record.PrivacyPolicyVersion = cfg.Privacy
		record.ServiceTermsVersion = cfg.Terms
	}
	return record, err
}
func settings(ctx context.Context, tx pgx.Tx, brand string) (Settings, error) {
	var raw []byte
	var cfg Settings
	err := tx.QueryRow(ctx, "SELECT auth_config FROM brands WHERE id=$1 AND status<>'disabled' FOR SHARE", brand).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &cfg)
	}
	return cfg, err
}
func accept(cfg Settings, privacy, terms string) bool {
	return privacy != "" && terms != "" && privacy == cfg.Privacy && terms == cfg.Terms
}
func (s *Store) issue(ctx context.Context, tx pgx.Tx, v View, meta Metadata, action string) (mutation.Result, error) {
	token, err := authcrypto.NewSessionToken()
	if err != nil {
		return mutation.Result{}, err
	}
	expiry := time.Now().UTC().Add(s.TTL)
	_, err = tx.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, ids.New(), tokenHash(token), v.User.ID, v.Member.ID, v.Member.BrandID, expiry)
	if err != nil {
		return mutation.Result{}, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: v.Member.BrandID, ActorType: "user", ActorID: v.User.ID, Action: action, ResourceType: "member", ResourceID: v.Member.ID, RequestID: meta.RequestID, IP: meta.IP, After: map[string]string{"status": v.Member.Status}})
	if err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(200, Authentication{View: v, AccessToken: token, TokenType: "Bearer", ExpiresAt: expiry}), nil
}
func (s *Store) Register(ctx context.Context, tx pgx.Tx, brand string, in RegisterInput, meta Metadata) (mutation.Result, error) {
	var err error
	if in.Username != "" {
		in.Username, err = normalizeUsername(in.Username)
		if err != nil {
			return mutation.Fail(400, "PROFILE_INVALID", "用户名须为 3-32 位字母、数字或下划线，并以字母开头"), nil
		}
	}
	if in.Phone != "" {
		in.Phone, err = normalizePhone(in.Phone)
		if err != nil {
			return mutation.Fail(400, "PROFILE_INVALID", "手机号须使用国际 E.164 格式"), nil
		}
	}
	if in.Username == "" && in.Phone == "" {
		return mutation.Fail(400, "PROFILE_INVALID", "需要用户名或手机号"), nil
	}
	cfg, err := settings(ctx, tx, brand)
	if err != nil {
		return mutation.Result{}, err
	}
	if !accept(cfg, in.Privacy, in.Terms) {
		return mutation.Fail(400, "TERMS_REQUIRED", "请接受当前品牌隐私条款和服务协议"), nil
	}
	hash, err := s.PasswordHash(ctx, in.Password)
	if err == ErrBusy {
		return mutation.Fail(429, "AUTH_BUSY", "登录繁忙，请稍后重试"), nil
	}
	if err == authcrypto.ErrInvalidPassword {
		return mutation.Fail(400, "PASSWORD_INVALID", "密码须为 10-128 字节"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	u := User{ID: ids.New(), Username: in.Username, Phone: in.Phone, Status: "active"}
	_, err = tx.Exec(ctx, `INSERT INTO global_users(id,username,phone,password_hash,username_set_at,phone_set_at) VALUES($1,NULLIF($2,''),NULLIF($3,''),$4,CASE WHEN $2<>'' THEN now() END,CASE WHEN $3<>'' THEN now() END)`, u.ID, u.Username, u.Phone, hash)
	if unique(err) {
		return mutation.Fail(409, "IDENTITY_EXISTS", "账号已存在；请登录以加入当前品牌"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	m, err := s.join(ctx, tx, brand, u.ID, in.Privacy, in.Terms, meta)
	if err != nil {
		return mutation.Result{}, err
	}
	result, err := s.issue(ctx, tx, View{u, m}, meta, "user.register")
	result.Status = 201
	return result, err
}
func (s *Store) join(ctx context.Context, tx pgx.Tx, brand, user, privacy, terms string, meta Metadata) (Member, error) {
	return s.createMember(ctx, tx, brand, user, privacy, terms, meta, memberCreateOptions{joinMethod: "domain", termsAccepted: true})
}

type memberCreateOptions struct {
	joinMethod    string
	createdBy     string
	displayName   string
	notes         string
	termsAccepted bool
}

func (s *Store) createMember(ctx context.Context, tx pgx.Tx, brand, user, privacy, terms string, meta Metadata, opts memberCreateOptions) (Member, error) {
	var m Member
	m.ID = ids.New()
	m.BrandID = brand
	err := tx.QueryRow(ctx, `INSERT INTO brand_members
		(id,brand_id,global_user_id,join_method,join_domain,privacy_policy_version,service_terms_version,
		 terms_accepted,accepted_at,created_by,display_name,notes)
		VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,CASE WHEN $8 THEN now() ELSE NULL END,NULLIF($9,'')::uuid,$10,$11)
		RETURNING status,display_name,joined_at`, m.ID, brand, user, opts.joinMethod, meta.Domain, privacy, terms, opts.termsAccepted, opts.createdBy, opts.displayName, opts.notes).Scan(&m.Status, &m.DisplayName, &m.JoinedAt)
	if err != nil {
		return m, err
	}
	account := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, account, brand, m.ID); err != nil {
		return m, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,source,state FROM unnest(ARRAY['recharge','winning','gift']) AS source CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) AS state`, brand, account)
	return m, err
}

// acceptPendingMember records the end user's current consent; administrative
// provisioning deliberately never calls this helper.
func acceptPendingMember(ctx context.Context, tx pgx.Tx, brand, user string, m *Member, cfg Settings, privacy, terms string, meta Metadata) (bool, error) {
	var accepted bool
	if err := tx.QueryRow(ctx, `SELECT terms_accepted FROM brand_members WHERE id=$1 AND brand_id=$2`, m.ID, brand).Scan(&accepted); err != nil {
		return false, err
	}
	if accepted {
		return true, nil
	}
	if !accept(cfg, privacy, terms) {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE brand_members SET terms_accepted=true,accepted_at=now(),privacy_policy_version=$2,service_terms_version=$3 WHERE id=$1`, m.ID, cfg.Privacy, cfg.Terms); err != nil {
		return false, err
	}
	if _, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "user", ActorID: user, Action: "member.terms_accept", ResourceType: "member", ResourceID: m.ID, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]bool{"terms_accepted": false}, After: map[string]any{"terms_accepted": true, "privacy_policy_version": cfg.Privacy, "service_terms_version": cfg.Terms}}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) Login(ctx context.Context, tx pgx.Tx, brand string, in LoginInput, meta Metadata) (mutation.Result, error) {
	cfg, err := settings(ctx, tx, brand)
	if err != nil {
		return mutation.Result{}, err
	}
	identifier := strings.ToLower(strings.TrimSpace(in.Identifier))
	var u User
	var hash string
	err = tx.QueryRow(ctx, `SELECT id::text,coalesce(username,''),coalesce(phone,''),coalesce(telegram_user_id,''),status,coalesce(password_hash,'') FROM global_users WHERE username=$1 OR phone=$1 FOR UPDATE`, identifier).Scan(&u.ID, &u.Username, &u.Phone, &u.TelegramID, &u.Status, &hash)
	found := err == nil
	if err != nil && err != pgx.ErrNoRows {
		return mutation.Result{}, err
	}
	if hash == "" {
		hash = s.dummyHash
	}
	valid, err := s.verify(ctx, in.Password, hash)
	if err == ErrBusy {
		return mutation.Fail(429, "AUTH_BUSY", "登录繁忙，请稍后重试"), nil
	}
	if !found || !valid || err != nil || u.Status != "active" {
		return mutation.Fail(401, "AUTH_INVALID_CREDENTIALS", "账号或密码不正确"), nil
	}
	var m Member
	err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,status,display_name,joined_at FROM brand_members WHERE brand_id=$1 AND global_user_id=$2 FOR UPDATE`, brand, u.ID).Scan(&m.ID, &m.BrandID, &m.Status, &m.DisplayName, &m.JoinedAt)
	if err == pgx.ErrNoRows {
		if !accept(cfg, in.Privacy, in.Terms) {
			return mutation.Fail(409, "BRAND_JOIN_REQUIRED", "请接受当前品牌条款后加入"), nil
		}
		m, err = s.join(ctx, tx, brand, u.ID, in.Privacy, in.Terms, meta)
	}
	if err != nil {
		return mutation.Result{}, err
	}
	accepted, err := acceptPendingMember(ctx, tx, brand, u.ID, &m, cfg, in.Privacy, in.Terms, meta)
	if err != nil {
		return mutation.Result{}, err
	}
	if !accepted {
		return mutation.Fail(409, "BRAND_JOIN_REQUIRED", "请接受当前品牌条款后加入"), nil
	}
	if m.Status != "normal" && m.Status != "frozen" {
		return mutation.Fail(403, "MEMBER_DISABLED", "当前品牌账号不可登录"), nil
	}
	return s.issue(ctx, tx, View{u, m}, meta, "user.login")
}
func (s *Store) Authenticate(ctx context.Context, brand, token string) (Session, error) {
	var v Session
	if len(token) != 43 {
		return v, ErrSession
	}
	err := s.DB.QueryRow(ctx, `SELECT s.id::text,u.id::text,coalesce(u.username,''),coalesce(u.phone,''),coalesce(u.telegram_user_id,''),u.status,m.id::text,m.brand_id::text,m.status,m.display_name,m.joined_at
 FROM sessions s JOIN global_users u ON u.id=s.user_id JOIN brand_members m ON m.id=s.member_id AND m.brand_id=s.brand_id JOIN brands b ON b.id=m.brand_id
 WHERE s.token_hash=$1 AND s.brand_id=$2 AND s.admin_id IS NULL AND s.revoked_at IS NULL AND s.expires_at>now() AND u.status='active' AND m.status IN ('normal','frozen') AND m.terms_accepted=true AND b.status<>'disabled'`, tokenHash(token), brand).Scan(&v.ID, &v.User.ID, &v.User.Username, &v.User.Phone, &v.User.TelegramID, &v.User.Status, &v.Member.ID, &v.Member.BrandID, &v.Member.Status, &v.Member.DisplayName, &v.Member.JoinedAt)
	if err == pgx.ErrNoRows {
		return v, ErrSession
	}
	return v, err
}
func (s *Store) Logout(ctx context.Context, tx pgx.Tx, brand, token string, meta Metadata) (mutation.Result, error) {
	var id, user, member string
	err := tx.QueryRow(ctx, "SELECT id::text,user_id::text,member_id::text FROM sessions WHERE token_hash=$1 AND brand_id=$2 AND admin_id IS NULL FOR UPDATE", tokenHash(token), brand).Scan(&id, &user, &member)
	if err == pgx.ErrNoRows {
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "会话不可用"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	if _, err = tx.Exec(ctx, "UPDATE sessions SET revoked_at=coalesce(revoked_at,now()) WHERE id=$1", id); err != nil {
		return mutation.Result{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "user", ActorID: user, Action: "user.logout", ResourceType: "member", ResourceID: member, RequestID: meta.RequestID, IP: meta.IP})
	return mutation.OK(200, map[string]string{"audit_log_id": auditID}), err
}
func (s *Store) FillProfile(ctx context.Context, tx pgx.Tx, brand, user string, in ProfileInput, meta Metadata) (mutation.Result, error) {
	var oldUsername, oldPhone string
	err := tx.QueryRow(ctx, "SELECT coalesce(username,''),coalesce(phone,'') FROM global_users WHERE id=$1 FOR UPDATE", user).Scan(&oldUsername, &oldPhone)
	if err != nil {
		return mutation.Result{}, err
	}
	if in.Username == "" && in.Phone == "" {
		return mutation.Fail(400, "PROFILE_INVALID", "没有需要补充的资料"), nil
	}
	if in.Username != "" {
		if oldUsername != "" {
			return mutation.Fail(409, "PROFILE_IMMUTABLE", "用户名填写后不能修改"), nil
		}
		in.Username, err = normalizeUsername(in.Username)
		if err != nil {
			return mutation.Fail(400, "PROFILE_INVALID", "用户名格式不正确"), nil
		}
	}
	if in.Phone != "" {
		if oldPhone != "" {
			return mutation.Fail(409, "PROFILE_IMMUTABLE", "手机号填写后不能修改"), nil
		}
		in.Phone, err = normalizePhone(in.Phone)
		if err != nil {
			return mutation.Fail(400, "PROFILE_INVALID", "手机号格式不正确"), nil
		}
	}
	_, err = tx.Exec(ctx, `UPDATE global_users SET username=coalesce(username,NULLIF($2,'')),phone=coalesce(phone,NULLIF($3,'')),username_set_at=CASE WHEN username IS NULL AND $2<>'' THEN now() ELSE username_set_at END,phone_set_at=CASE WHEN phone IS NULL AND $3<>'' THEN now() ELSE phone_set_at END,updated_at=now() WHERE id=$1`, user, in.Username, in.Phone)
	if unique(err) {
		return mutation.Fail(409, "IDENTITY_EXISTS", "用户名或手机号已被使用"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "user", ActorID: user, Action: "user.profile_fill", ResourceType: "user", ResourceID: user, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]bool{"username_present": oldUsername != "", "phone_present": oldPhone != ""}, After: map[string]bool{"username_present": oldUsername != "" || in.Username != "", "phone_present": oldPhone != "" || in.Phone != ""}})
	return mutation.OK(200, map[string]string{"audit_log_id": auditID}), err
}
