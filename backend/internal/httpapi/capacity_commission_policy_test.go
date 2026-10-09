//go:build capacity && !windows

package httpapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type capacityCommissionBrand struct {
	ID   string
	Code string
}

func capacityCommissionBrands(ctx context.Context, f capacityFixture) ([]capacityCommissionBrand, error) {
	codes := make([]string, 0, 2)
	seen := make(map[string]bool, 2)
	for _, user := range f.Users {
		if !seen[user.Brand] {
			seen[user.Brand] = true
			codes = append(codes, user.Brand)
		}
	}
	rows, err := f.DB.Query(ctx, `SELECT b.id::text,b.code
		FROM brands b
		JOIN brand_agent_policies ap ON ap.brand_id=b.id
		WHERE b.code=ANY($1::text[]) AND ap.config->'enabled'='true'::jsonb
		ORDER BY b.code`, codes)
	if err != nil {
		return nil, fmt.Errorf("query agency-enabled capacity brands: %w", err)
	}
	defer rows.Close()
	brands := make([]capacityCommissionBrand, 0, 2)
	for rows.Next() {
		var brand capacityCommissionBrand
		if err = rows.Scan(&brand.ID, &brand.Code); err != nil {
			return nil, fmt.Errorf("read agency-enabled capacity brand: %w", err)
		}
		brands = append(brands, brand)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read agency-enabled capacity brands: %w", err)
	}
	if len(brands) != 2 {
		return nil, fmt.Errorf("found %d agency-enabled capacity brands, want 2", len(brands))
	}
	return brands, nil
}

func prepareCapacityCommission(t *testing.T, f capacityFixture) time.Time {
	t.Helper()
	ctx := context.Background()
	brands, err := capacityCommissionBrands(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	var dbNow time.Time
	if err = f.DB.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dbNow); err != nil {
		t.Fatalf("read database clock for commission boundary: %v", err)
	}
	dbNow = dbNow.UTC()
	boundary := dbNow.Add(25 * time.Second).Truncate(time.Second)
	if !boundary.After(dbNow) {
		t.Fatalf("commission boundary %s is not after database time %s", boundary, dbNow)
	}
	weekday := int(boundary.Weekday())
	calendar := commission.Calendar{
		Timezone: "UTC", Cycle: "weekly", BoundaryTime: boundary.Format("15:04:05"), Weekday: &weekday,
	}
	financial := commission.Service{DB: f.DB}
	permissions := []string{
		"commission.view.brand",
		"commission_policy.write.brand",
		"commission_payment_policy.write.brand",
	}
	for _, brand := range brands {
		adminID, roleID := ids.New(), ids.New()
		capacityTx(t, f.DB, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash)
				VALUES($1,$2,'capacity-fixture-only-no-login')`, adminID, "capacity_commission_"+adminID); err != nil {
				return fmt.Errorf("create commission admin for %s: %w", brand.Code, err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name)
				VALUES($1,$2,$3,'Capacity commission operator')`, roleID, brand.ID, "capacity_commission_"+roleID); err != nil {
				return fmt.Errorf("create commission role for %s: %w", brand.Code, err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, adminID, brand.ID); err != nil {
				return fmt.Errorf("scope commission admin to %s: %w", brand.Code, err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, adminID, roleID); err != nil {
				return fmt.Errorf("assign commission role for %s: %w", brand.Code, err)
			}
			var catalogCount int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE key=ANY($1::text[])`, permissions).Scan(&catalogCount); err != nil {
				return fmt.Errorf("query commission permission catalog for %s: %w", brand.Code, err)
			}
			if catalogCount != len(permissions) {
				return fmt.Errorf("brand %s has %d of %d required commission permissions in catalog", brand.Code, catalogCount, len(permissions))
			}
			if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key)
				SELECT $1,key FROM permissions WHERE key=ANY($2::text[])`, roleID, permissions); err != nil {
				return fmt.Errorf("grant commission permissions to %s: %w", brand.Code, err)
			}
			var grantCount int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE role_id=$1 AND permission_key=ANY($2::text[])`, roleID, permissions).Scan(&grantCount); err != nil {
				return fmt.Errorf("query persisted commission grants for %s: %w", brand.Code, err)
			}
			if grantCount != len(permissions) {
				return fmt.Errorf("brand %s has %d of %d required commission grants", brand.Code, grantCount, len(permissions))
			}
			return nil
		})

		actor, err := (adminsys.Store{DB: f.DB}).Account(ctx, adminID)
		if err != nil {
			t.Fatalf("load persisted commission admin for %s: %v", brand.Code, err)
		}
		if len(actor.BrandIDs) != 1 || actor.BrandIDs[0] != brand.ID {
			t.Fatalf("persisted commission admin for %s has scopes %v, want only %s", brand.Code, actor.BrandIDs, brand.ID)
		}
		for _, permission := range []struct{ resource, action string }{
			{"commission", "view"},
			{"commission_policy", "write"},
			{"commission_payment_policy", "write"},
		} {
			if !access.Authorize(actor, permission.resource, permission.action, access.ScopeBrand, brand.ID) {
				t.Fatalf("persisted commission admin for %s lacks %s.%s.brand", brand.Code, permission.resource, permission.action)
			}
		}

		policy, err := financial.Policy(ctx, brand.ID)
		if err != nil {
			t.Fatalf("read commission policy for %s: %v", brand.Code, err)
		}
		capacityTx(t, f.DB, func(tx pgx.Tx) error {
			if _, err = financial.Update(ctx, tx, brand.ID, actor, commission.PolicyInput{
				Version: policy.Version,
				Config: commission.PolicyConfig{
					Enabled: true, Calendar: &calendar, PayoutMode: commission.PayoutAutomatic,
				},
				Reason: "enable weekly automatic commission for capacity test",
			}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()}); err != nil {
				return fmt.Errorf("enable commission policy for %s: %w", brand.Code, err)
			}
			paymentPolicy, err := financial.PaymentPolicyTx(ctx, tx, brand.ID)
			if err != nil {
				return fmt.Errorf("read commission payment policy for %s: %w", brand.Code, err)
			}
			if _, err = financial.UpdatePaymentPolicyTx(ctx, tx, brand.ID, actor, commission.PaymentPolicyInput{
				Version: paymentPolicy.Version, Enabled: true,
				Reason: "enable automatic commission payments for capacity test",
			}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()}); err != nil {
				return fmt.Errorf("enable commission payments for %s: %w", brand.Code, err)
			}
			return nil
		})
	}
	var dbAfter time.Time
	if err = f.DB.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dbAfter); err != nil {
		t.Fatalf("recheck commission boundary against database clock: %v", err)
	}
	if !boundary.After(dbAfter.UTC()) {
		t.Fatalf("commission boundary %s was not still future after policy setup (database time %s)", boundary, dbAfter.UTC())
	}
	return boundary.UTC()
}

func processCapacityCommission(ctx context.Context, f capacityFixture) error {
	brands, err := capacityCommissionBrands(ctx, f)
	if err != nil {
		return err
	}
	brandIDs := make([]string, 0, len(brands))
	for _, brand := range brands {
		brandIDs = append(brandIDs, brand.ID)
	}
	service := commission.Service{DB: f.DB}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		discovered, err := service.ProcessDiscovery(ctx, 100)
		if err != nil {
			return fmt.Errorf("process commission discovery: %w", err)
		}
		cycles, err := service.ProcessCycles(ctx, 100)
		if err != nil {
			return fmt.Errorf("process commission cycles: %w", err)
		}
		payments, err := service.ProcessPayments(ctx, 100)
		if err != nil {
			return fmt.Errorf("process commission payments: %w", err)
		}

		rows, err := f.DB.Query(ctx, `SELECT 'discovery'::text,id::text,state,last_error_code
			FROM commission_discovery WHERE brand_id::text=ANY($1::text[]) AND state='failed'
			UNION ALL
			SELECT 'cycle'::text,id::text,state,last_error_code
			FROM commission_cycles WHERE brand_id::text=ANY($1::text[]) AND state='failed'
			UNION ALL
			SELECT 'payment'::text,id::text,state,last_error_code
			FROM commission_payments WHERE brand_id::text=ANY($1::text[]) AND state IN('failed','blocked','stale')
			LIMIT 1`, brandIDs)
		if err != nil {
			return fmt.Errorf("check commission worker failures: %w", err)
		}
		if rows.Next() {
			var kind, id, state, code string
			if err = rows.Scan(&kind, &id, &state, &code); err != nil {
				rows.Close()
				return fmt.Errorf("read commission worker failure: %w", err)
			}
			rows.Close()
			return fmt.Errorf("commission %s %s entered state %s with code %s", kind, id, state, code)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("read commission worker failures: %w", err)
		}
		rows.Close()

		var paid int
		if err = f.DB.QueryRow(ctx, `SELECT count(*) FROM commission_payments
			WHERE brand_id::text=ANY($1::text[]) AND state='paid'`, brandIDs).Scan(&paid); err != nil {
			return fmt.Errorf("count completed commission payments: %w", err)
		}
		if paid == 2 {
			return nil
		}
		if paid > 2 {
			return fmt.Errorf("commission workers completed %d payments, want exactly 2", paid)
		}
		if discovered == 0 && cycles == 0 && payments == 0 {
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}
