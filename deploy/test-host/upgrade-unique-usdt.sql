
-- Shared-address USDT orders: principal is amount; payable is principal + fee.
ALTER TABLE cash_orders ADD COLUMN automatic boolean NOT NULL DEFAULT false;
ALTER TABLE cash_orders ADD COLUMN sender text NOT NULL DEFAULT '';
CREATE TABLE cash_usdt_amounts (
 address text NOT NULL,payable bigint NOT NULL CHECK(payable>0),order_id uuid NOT NULL UNIQUE REFERENCES cash_orders(id),
 PRIMARY KEY(address,payable)
);
CREATE TRIGGER cash_usdt_amounts_immutable BEFORE UPDATE OR DELETE ON cash_usdt_amounts FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE cash_usdt_receipts (
 transaction_id text NOT NULL,event_index integer NOT NULL CHECK(event_index>=0),order_id uuid NOT NULL UNIQUE REFERENCES cash_orders(id),
 sender text NOT NULL,received bigint NOT NULL CHECK(received>0),block_time timestamptz NOT NULL,
 PRIMARY KEY(transaction_id,event_index)
);
CREATE TRIGGER cash_usdt_receipts_immutable BEFORE UPDATE OR DELETE ON cash_usdt_receipts FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE cash_orders ADD COLUMN event_index integer NOT NULL DEFAULT -1 CHECK(event_index>=-1);
DROP INDEX cash_deposit_reference;
CREATE UNIQUE INDEX cash_deposit_reference ON cash_orders(brand_id,currency,reference) WHERE kind='deposit' AND currency<>'USDT' AND state='confirmed' AND reference<>'';
DROP INDEX cash_usdt_deposit_transaction;
CREATE UNIQUE INDEX cash_usdt_deposit_transaction ON cash_orders(reference,event_index) WHERE kind='deposit' AND currency='USDT' AND state='confirmed' AND reference<>'';
