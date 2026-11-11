-- 0006 guarantee code formulas + warranty counted from the manufacture date.

-- A product can describe its guarantee codes as a formula (list of tokens: text / number / year / month / day / serial).
ALTER TABLE products ADD COLUMN code_formula JSONB NOT NULL DEFAULT '[]';
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_code_format_check;
ALTER TABLE products ADD CONSTRAINT products_code_format_check CHECK (code_format IN ('simple','jalali_encoded','evinki_vacuum','any','formula'));

-- What the guarantee period is counted from: the manufacture date read from the code, the purchase date,
-- or whichever gives the customer the later expiry ("best").
ALTER TABLE products ADD COLUMN warranty_basis TEXT NOT NULL DEFAULT 'manufacture' CHECK (warranty_basis IN ('manufacture','purchase','best'));

ALTER TABLE guarantees ADD COLUMN manufacture_date DATE;
ALTER TABLE guarantees ADD COLUMN manufacture_approx BOOLEAN NOT NULL DEFAULT FALSE; -- only the month/season was known
ALTER TABLE guarantees ADD COLUMN expiry_basis TEXT NOT NULL DEFAULT 'purchase';     -- what the expiry was counted from
ALTER TABLE guarantees ADD COLUMN expiry_edited_at TIMESTAMPTZ;                        -- an admin changed the date by hand
ALTER TABLE guarantees ADD COLUMN expiry_edit_note TEXT NOT NULL DEFAULT '';
