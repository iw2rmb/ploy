# DNS Update Guide for *.dev.ployd.app SSL

## Required DNS Changes

**Domain:** ployd.app  
**Target IP:** 45.12.75.241  
**Purpose:** Enable *.dev.ployd.app wildcard certificate

## Step 1: Login to Namecheap

1. Go to [Namecheap.com](https://www.namecheap.com)
2. Login to your account
3. Go to **Domain List**
4. Find **ployd.app** and click **Manage**
5. Click **Advanced DNS** tab

## Step 2: Update/Add A Records

**Add or modify these A records:**

| Type | Host | Value | TTL |
|------|------|-------|-----|
| A | dev | 45.12.75.241 | 300 |
| A | *.dev | 45.12.75.241 | 300 |

### Detailed Instructions:

1. **For `dev` record:**
   - Click **Add New Record** (or edit existing)
   - Type: **A Record**
   - Host: **dev**
   - Value: **45.12.75.241**
   - TTL: **300** (5 minutes)

2. **For `*.dev` wildcard record:**
   - Click **Add New Record** (or edit existing)
   - Type: **A Record** 
   - Host: **\*.dev**
   - Value: **45.12.75.241**
   - TTL: **300** (5 minutes)

3. **Save changes**

## Step 3: Verify DNS Update

After saving, the records should look like:
```
dev.ployd.app → 45.12.75.241
*.dev.ployd.app → 45.12.75.241
```

## Step 4: Test DNS Propagation

Wait 5-10 minutes, then run:
```bash
dig +short dev.ployd.app
dig +short api.dev.ployd.app
```

Both should return: **45.12.75.241**

## What This Enables

- **api.dev.ployd.app** → Controller endpoint
- **myapp.dev.ployd.app** → User applications  
- **{any-app}.dev.ployd.app** → All dev apps

## Next Steps

Once DNS propagation completes:
1. Configure Namecheap API credentials
2. Run SSL deployment script
3. Enjoy HTTPS for all dev applications!