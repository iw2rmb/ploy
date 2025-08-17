const express = require('express');
const app = express();
app.get('/healthz', (req,res)=>res.send('ok'));
app.get('/', (req,res)=>res.send('hello from node-apisvc'));
app.listen(8080, ()=>console.log('listening on 8080'));
