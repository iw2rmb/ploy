# Namecheap API Setup for SSL Wildcard Certificate

## Required for Let's Encrypt DNS-01 Challenge

The SSL wildcard certificate requires Namecheap API access to create DNS challenge records automatically.

## Step 1: Enable API Access

1. Login to [Namecheap Account](https://ap.www.namecheap.com)
2. Go to **Profile** → **Tools** → **API Access**
3. Turn **API Access** to **ON**
4. Note down:
   - **API Key** (long string)
   - **Username** (your Namecheap username)

## Step 2: Whitelist VPS IP

**CRITICAL:** Namecheap requires IP whitelisting for API access.

1. In the same **API Access** page
2. Add **45.12.75.241** to **Whitelisted IPs**
3. Save changes

## Step 3: Get Credentials

You'll need these three values:
```bash
NAMECHEAP_API_USER="your-username"     # Usually same as username
NAMECHEAP_API_KEY="your-api-key"       # Long API key from step 1
NAMECHEAP_USERNAME="your-username"     # Your Namecheap username
```

## Step 4: Test API Access (Optional)

From the VPS, test API access:
```bash
curl "https://api.namecheap.com/xml.response?ApiUser=USERNAME&ApiKey=APIKEY&UserName=USERNAME&Command=namecheap.domains.getList&ClientIp=45.12.75.241"
```

## Step 5: Set Environment Variables

On the VPS, set these environment variables:
```bash
export NAMECHEAP_API_USER="your-username"
export NAMECHEAP_API_KEY="your-api-key" 
export NAMECHEAP_USERNAME="your-username"
export CERT_EMAIL="admin@ployd.app"
```

## Security Notes

- API key provides full DNS control
- Keep credentials secure
- VPS IP must be whitelisted
- API calls only work from whitelisted IPs

## What This Enables

- Automatic DNS challenge record creation
- Let's Encrypt wildcard certificate provisioning
- No manual certificate management required
- Automatic 90-day renewal