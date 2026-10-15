DROP INDEX IF EXISTS users_org_phone_key;
ALTER TABLE users DROP COLUMN IF EXISTS phone_verified;

DROP INDEX IF EXISTS providers_category_idx;
ALTER TABLE providers
    DROP COLUMN IF EXISTS ssl_mode,
    DROP COLUMN IF EXISTS from_address,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS content,
    DROP COLUMN IF EXISTS receiver,
    DROP COLUMN IF EXISTS region_id,
    DROP COLUMN IF EXISTS sign_name,
    DROP COLUMN IF EXISTS template_code,
    DROP COLUMN IF EXISTS endpoint,
    DROP COLUMN IF EXISTS domain,
    DROP COLUMN IF EXISTS bucket,
    DROP COLUMN IF EXISTS path_prefix;

ALTER TABLE applications
    DROP COLUMN IF EXISTS logo_dark,
    DROP COLUMN IF EXISTS favicon,
    DROP COLUMN IF EXISTS tags,
    DROP COLUMN IF EXISTS enable_link_with_email,
    DROP COLUMN IF EXISTS org_choice_mode,
    DROP COLUMN IF EXISTS ip_restriction,
    DROP COLUMN IF EXISTS ip_whitelist,
    DROP COLUMN IF EXISTS signup_url,
    DROP COLUMN IF EXISTS signin_url,
    DROP COLUMN IF EXISTS forget_url,
    DROP COLUMN IF EXISTS affiliation_url,
    DROP COLUMN IF EXISTS terms_of_use,
    DROP COLUMN IF EXISTS forced_redirect_origin,
    DROP COLUMN IF EXISTS backchannel_logout_uri,
    DROP COLUMN IF EXISTS token_fields,
    DROP COLUMN IF EXISTS disable_refresh_rotation,
    DROP COLUMN IF EXISTS signin_methods,
    DROP COLUMN IF EXISTS signup_items,
    DROP COLUMN IF EXISTS signin_items;

