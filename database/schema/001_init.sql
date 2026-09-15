CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;

CREATE TABLE customers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cif_number varchar(20) NOT NULL,
    nik varchar(32) NOT NULL,
    full_name varchar(200) NOT NULL,
    date_of_birth date,
    email varchar(320),
    phone_number varchar(32),
    status varchar(20) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_customers_cif_number UNIQUE (cif_number),
    CONSTRAINT uq_customers_nik UNIQUE (nik),
    CONSTRAINT chk_customers_cif_number_not_blank CHECK (btrim(cif_number) <> ''),
    CONSTRAINT chk_customers_nik_not_blank CHECK (btrim(nik) <> ''),
    CONSTRAINT chk_customers_status CHECK (status IN ('ACTIVE', 'INACTIVE', 'BLOCKED', 'CLOSED'))
);

CREATE TABLE accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_number varchar(34) NOT NULL,
    customer_id uuid NOT NULL,
    product_code varchar(30) NOT NULL,
    currency_code char(3) NOT NULL,
    account_name varchar(200) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'PENDING',
    opened_at timestamptz,
    closed_at timestamptz,
    available_balance numeric(19, 2) NOT NULL DEFAULT 0,
    ledger_balance numeric(19, 2) NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_accounts_account_number UNIQUE (account_number),
    CONSTRAINT fk_accounts_customer
        FOREIGN KEY (customer_id) REFERENCES customers (id) ON DELETE RESTRICT,
    CONSTRAINT chk_accounts_number_not_blank CHECK (btrim(account_number) <> ''),
    CONSTRAINT chk_accounts_product_not_blank CHECK (btrim(product_code) <> ''),
    CONSTRAINT chk_accounts_currency CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_accounts_name_not_blank CHECK (btrim(account_name) <> ''),
    CONSTRAINT chk_accounts_status CHECK (status IN ('PENDING', 'ACTIVE', 'DORMANT', 'BLOCKED', 'CLOSED')),
    CONSTRAINT chk_accounts_closed_at CHECK (closed_at IS NULL OR opened_at IS NULL OR closed_at >= opened_at)
);

CREATE TABLE internet_banking_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id uuid NOT NULL,
    username varchar(100) NOT NULL,
    password_hash text NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'ACTIVE',
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_internet_banking_users_customer UNIQUE (customer_id),
    CONSTRAINT uq_internet_banking_users_username UNIQUE (username),
    CONSTRAINT fk_internet_banking_users_customer
        FOREIGN KEY (customer_id) REFERENCES customers (id) ON DELETE RESTRICT,
    CONSTRAINT chk_internet_banking_users_username_not_blank CHECK (btrim(username) <> ''),
    CONSTRAINT chk_internet_banking_users_password_hash_not_blank CHECK (btrim(password_hash) <> ''),
    CONSTRAINT chk_internet_banking_users_status CHECK (status IN ('ACTIVE', 'LOCKED', 'SUSPENDED', 'DISABLED'))
);

CREATE TABLE mobile_banking_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id uuid NOT NULL,
    mobile_number varchar(32) NOT NULL,
    pin_hash text NOT NULL,
    device_binding_id varchar(255),
    status varchar(20) NOT NULL DEFAULT 'ACTIVE',
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_mobile_banking_users_customer UNIQUE (customer_id),
    CONSTRAINT uq_mobile_banking_users_mobile_number UNIQUE (mobile_number),
    CONSTRAINT fk_mobile_banking_users_customer
        FOREIGN KEY (customer_id) REFERENCES customers (id) ON DELETE RESTRICT,
    CONSTRAINT chk_mobile_banking_users_mobile_number_not_blank CHECK (btrim(mobile_number) <> ''),
    CONSTRAINT chk_mobile_banking_users_pin_hash_not_blank CHECK (btrim(pin_hash) <> ''),
    CONSTRAINT chk_mobile_banking_users_status CHECK (status IN ('ACTIVE', 'LOCKED', 'SUSPENDED', 'DISABLED'))
);

CREATE TABLE transactions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_reference varchar(64) NOT NULL,
    idempotency_key varchar(128) NOT NULL,
    transaction_type varchar(30) NOT NULL,
    channel varchar(30) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'PENDING',
    currency_code char(3) NOT NULL,
    amount numeric(19, 2) NOT NULL,
    description varchar(500),
    initiated_by_customer_id uuid,
    internet_banking_user_id uuid,
    mobile_banking_user_id uuid,
    occurred_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    posted_at timestamptz,
    reversal_of_transaction_id uuid,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_transactions_reference UNIQUE (transaction_reference),
    CONSTRAINT uq_transactions_channel_idempotency UNIQUE (channel, idempotency_key),
    CONSTRAINT fk_transactions_initiated_by_customer
        FOREIGN KEY (initiated_by_customer_id) REFERENCES customers (id) ON DELETE RESTRICT,
    CONSTRAINT fk_transactions_internet_banking_user
        FOREIGN KEY (internet_banking_user_id) REFERENCES internet_banking_users (id) ON DELETE RESTRICT,
    CONSTRAINT fk_transactions_mobile_banking_user
        FOREIGN KEY (mobile_banking_user_id) REFERENCES mobile_banking_users (id) ON DELETE RESTRICT,
    CONSTRAINT fk_transactions_reversal
        FOREIGN KEY (reversal_of_transaction_id) REFERENCES transactions (id) ON DELETE RESTRICT,
    CONSTRAINT chk_transactions_reference_not_blank CHECK (btrim(transaction_reference) <> ''),
    CONSTRAINT chk_transactions_idempotency_key_not_blank CHECK (btrim(idempotency_key) <> ''),
    CONSTRAINT chk_transactions_type_not_blank CHECK (btrim(transaction_type) <> ''),
    CONSTRAINT chk_transactions_channel CHECK (channel IN ('BRANCH', 'INTERNET_BANKING', 'MOBILE_BANKING', 'SYSTEM')),
    CONSTRAINT chk_transactions_status CHECK (status IN ('PENDING', 'PROCESSING', 'POSTED', 'REVERSED', 'FAILED')),
    CONSTRAINT chk_transactions_currency CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_transactions_amount_positive CHECK (amount > 0),
    CONSTRAINT chk_transactions_metadata_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT chk_transactions_origin_identity CHECK (
        NOT (internet_banking_user_id IS NOT NULL AND mobile_banking_user_id IS NOT NULL)
    ),
    CONSTRAINT chk_transactions_posted_at CHECK (
        (status IN ('POSTED', 'REVERSED') AND posted_at IS NOT NULL)
        OR (status NOT IN ('POSTED', 'REVERSED') AND posted_at IS NULL)
    ),
    CONSTRAINT chk_transactions_not_self_reversal CHECK (
        reversal_of_transaction_id IS NULL OR reversal_of_transaction_id <> id
    )
);

CREATE TABLE transaction_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id uuid NOT NULL,
    account_id uuid NOT NULL,
    role varchar(20) NOT NULL,
    amount numeric(19, 2) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_transaction_accounts_transaction_account_role UNIQUE (transaction_id, account_id, role),
    CONSTRAINT fk_transaction_accounts_transaction
        FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE RESTRICT,
    CONSTRAINT fk_transaction_accounts_account
        FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE RESTRICT,
    CONSTRAINT chk_transaction_accounts_role CHECK (role IN ('DEBIT', 'CREDIT', 'FEE', 'SETTLEMENT')),
    CONSTRAINT chk_transaction_accounts_amount_positive CHECK (amount > 0)
);

CREATE TABLE journal_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_number varchar(64) NOT NULL,
    transaction_id uuid NOT NULL,
    entry_type varchar(30) NOT NULL DEFAULT 'NORMAL',
    status varchar(20) NOT NULL DEFAULT 'DRAFT',
    entry_date date NOT NULL,
    description varchar(500),
    reversal_of_journal_entry_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    posted_at timestamptz,
    CONSTRAINT uq_journal_entries_journal_number UNIQUE (journal_number),
    CONSTRAINT uq_journal_entries_transaction UNIQUE (transaction_id),
    CONSTRAINT fk_journal_entries_transaction
        FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE RESTRICT,
    CONSTRAINT fk_journal_entries_reversal
        FOREIGN KEY (reversal_of_journal_entry_id) REFERENCES journal_entries (id) ON DELETE RESTRICT,
    CONSTRAINT chk_journal_entries_number_not_blank CHECK (btrim(journal_number) <> ''),
    CONSTRAINT chk_journal_entries_type CHECK (entry_type IN ('NORMAL', 'REVERSAL', 'ADJUSTMENT')),
    CONSTRAINT chk_journal_entries_status CHECK (status IN ('DRAFT', 'POSTED', 'REVERSED')),
    CONSTRAINT chk_journal_entries_posted_at CHECK (
        (status IN ('POSTED', 'REVERSED') AND posted_at IS NOT NULL)
        OR (status NOT IN ('POSTED', 'REVERSED') AND posted_at IS NULL)
    ),
    CONSTRAINT chk_journal_entries_not_self_reversal CHECK (
        reversal_of_journal_entry_id IS NULL OR reversal_of_journal_entry_id <> id
    )
);

CREATE TABLE journal_lines (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_entry_id uuid NOT NULL,
    line_number smallint NOT NULL,
    account_id uuid NOT NULL,
    entry_side varchar(6) NOT NULL,
    amount numeric(19, 2) NOT NULL,
    currency_code char(3) NOT NULL,
    description varchar(500),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_journal_lines_entry_line_number UNIQUE (journal_entry_id, line_number),
    CONSTRAINT fk_journal_lines_journal_entry
        FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id) ON DELETE RESTRICT,
    CONSTRAINT fk_journal_lines_account
        FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE RESTRICT,
    CONSTRAINT chk_journal_lines_line_number_positive CHECK (line_number > 0),
    CONSTRAINT chk_journal_lines_entry_side CHECK (entry_side IN ('DEBIT', 'CREDIT')),
    CONSTRAINT chk_journal_lines_amount_positive CHECK (amount > 0),
    CONSTRAINT chk_journal_lines_currency CHECK (currency_code ~ '^[A-Z]{3}$')
);

CREATE TABLE ledger_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ledger_reference varchar(64) NOT NULL,
    account_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    journal_entry_id uuid NOT NULL,
    journal_line_id uuid NOT NULL,
    posting_type varchar(6) NOT NULL,
    amount numeric(19, 2) NOT NULL,
    currency_code char(3) NOT NULL,
    balance_after numeric(19, 2) NOT NULL,
    effective_at timestamptz NOT NULL,
    posted_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_ledger_entries_ledger_reference UNIQUE (ledger_reference),
    CONSTRAINT uq_ledger_entries_journal_line UNIQUE (journal_line_id),
    CONSTRAINT fk_ledger_entries_account
        FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE RESTRICT,
    CONSTRAINT fk_ledger_entries_transaction
        FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE RESTRICT,
    CONSTRAINT fk_ledger_entries_journal_entry
        FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id) ON DELETE RESTRICT,
    CONSTRAINT fk_ledger_entries_journal_line
        FOREIGN KEY (journal_line_id) REFERENCES journal_lines (id) ON DELETE RESTRICT,
    CONSTRAINT chk_ledger_entries_reference_not_blank CHECK (btrim(ledger_reference) <> ''),
    CONSTRAINT chk_ledger_entries_posting_type CHECK (posting_type IN ('DEBIT', 'CREDIT')),
    CONSTRAINT chk_ledger_entries_amount_positive CHECK (amount > 0),
    CONSTRAINT chk_ledger_entries_currency CHECK (currency_code ~ '^[A-Z]{3}$')
);

CREATE FUNCTION prevent_ledger_entry_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries are append-only; % is not permitted', TG_OP;
END;
$$;

CREATE TRIGGER trg_customers_set_updated_at
BEFORE UPDATE ON customers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_accounts_set_updated_at
BEFORE UPDATE ON accounts
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_internet_banking_users_set_updated_at
BEFORE UPDATE ON internet_banking_users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_mobile_banking_users_set_updated_at
BEFORE UPDATE ON mobile_banking_users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_transactions_set_updated_at
BEFORE UPDATE ON transactions
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_journal_entries_set_updated_at
BEFORE UPDATE ON journal_entries
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_ledger_entries_append_only
BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW EXECUTE FUNCTION prevent_ledger_entry_mutation();

CREATE INDEX idx_accounts_customer_status ON accounts (customer_id, status);

CREATE INDEX idx_transactions_customer_occurred_at
    ON transactions (initiated_by_customer_id, occurred_at DESC);

CREATE INDEX idx_transactions_status_occurred_at
    ON transactions (status, occurred_at);

CREATE INDEX idx_transactions_reversal_of_transaction
    ON transactions (reversal_of_transaction_id)
    WHERE reversal_of_transaction_id IS NOT NULL;

CREATE INDEX idx_transaction_accounts_account_transaction
    ON transaction_accounts (account_id, transaction_id);

CREATE INDEX idx_journal_entries_transaction ON journal_entries (transaction_id);

CREATE INDEX idx_journal_entries_reversal_of_journal_entry
    ON journal_entries (reversal_of_journal_entry_id)
    WHERE reversal_of_journal_entry_id IS NOT NULL;

CREATE INDEX idx_journal_lines_account_journal_entry
    ON journal_lines (account_id, journal_entry_id);

CREATE INDEX idx_ledger_entries_account_posted_at
    ON ledger_entries (account_id, posted_at DESC, id DESC);

CREATE INDEX idx_ledger_entries_transaction ON ledger_entries (transaction_id);

CREATE INDEX idx_ledger_entries_journal_entry ON ledger_entries (journal_entry_id);
