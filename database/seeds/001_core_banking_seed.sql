-- Development-only, deterministic Core Banking seed data.
-- Password and PIN values below are deliberately fake and must never be used outside local development.

INSERT INTO customers (
    id,
    cif_number,
    nik,
    full_name,
    date_of_birth,
    email,
    phone_number,
    status,
    created_at,
    updated_at
)
VALUES
    (
        '00000000-0000-4000-8000-000000000001',
        'CIF-TEST-000001',
        '0000000000000001',
        'Customer A (Fictional)',
        DATE '1990-01-15',
        'customer.a@example.test',
        '+620000000001',
        'ACTIVE',
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        TIMESTAMPTZ '2026-01-01 00:00:00+00'
    ),
    (
        '00000000-0000-4000-8000-000000000002',
        'CIF-TEST-000002',
        '0000000000000002',
        'Customer B (Fictional)',
        DATE '1992-02-20',
        'customer.b@example.test',
        '+620000000002',
        'ACTIVE',
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        TIMESTAMPTZ '2026-01-01 00:00:00+00'
    )
ON CONFLICT (id) DO NOTHING;

INSERT INTO accounts (
    id,
    account_number,
    customer_id,
    product_code,
    currency_code,
    account_name,
    status,
    opened_at,
    available_balance,
    ledger_balance,
    created_at,
    updated_at
)
VALUES
    (
        '00000000-0000-4000-8000-000000000101',
        'AURORA-TEST-000001',
        '00000000-0000-4000-8000-000000000001',
        'SAVINGS',
        'IDR',
        'Customer A Fictional Savings',
        'ACTIVE',
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        500000.00,
        500000.00,
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        TIMESTAMPTZ '2026-01-01 00:00:00+00'
    ),
    (
        '00000000-0000-4000-8000-000000000102',
        'AURORA-TEST-000002',
        '00000000-0000-4000-8000-000000000002',
        'SAVINGS',
        'IDR',
        'Customer B Fictional Savings',
        'ACTIVE',
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        100000.00,
        100000.00,
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        TIMESTAMPTZ '2026-01-01 00:00:00+00'
    )
ON CONFLICT (id) DO NOTHING;

INSERT INTO internet_banking_users (
    id,
    customer_id,
    username,
    password_hash,
    status,
    created_at,
    updated_at
)
VALUES
    (
        '00000000-0000-4000-8000-000000000201',
        '00000000-0000-4000-8000-000000000001',
        'fictional.customer.a',
        'DEV_ONLY_FAKE_PASSWORD_HASH_CUSTOMER_A_DO_NOT_USE',
        'ACTIVE',
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        TIMESTAMPTZ '2026-01-01 00:00:00+00'
    ),
    (
        '00000000-0000-4000-8000-000000000202',
        '00000000-0000-4000-8000-000000000002',
        'fictional.customer.b',
        'DEV_ONLY_FAKE_PASSWORD_HASH_CUSTOMER_B_DO_NOT_USE',
        'ACTIVE',
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        TIMESTAMPTZ '2026-01-01 00:00:00+00'
    )
ON CONFLICT (id) DO NOTHING;

INSERT INTO mobile_banking_users (
    id,
    customer_id,
    mobile_number,
    pin_hash,
    device_binding_id,
    status,
    created_at,
    updated_at
)
VALUES
    (
        '00000000-0000-4000-8000-000000000301',
        '00000000-0000-4000-8000-000000000001',
        '+620000000001',
        'DEV_ONLY_FAKE_PIN_HASH_CUSTOMER_A_DO_NOT_USE',
        'DEV-DEVICE-CUSTOMER-A',
        'ACTIVE',
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        TIMESTAMPTZ '2026-01-01 00:00:00+00'
    ),
    (
        '00000000-0000-4000-8000-000000000302',
        '00000000-0000-4000-8000-000000000002',
        '+620000000002',
        'DEV_ONLY_FAKE_PIN_HASH_CUSTOMER_B_DO_NOT_USE',
        'DEV-DEVICE-CUSTOMER-B',
        'ACTIVE',
        TIMESTAMPTZ '2026-01-01 00:00:00+00',
        TIMESTAMPTZ '2026-01-01 00:00:00+00'
    )
ON CONFLICT (id) DO NOTHING;
