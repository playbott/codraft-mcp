@echo off
echo [BUILD] Checking code with go vet...
go vet ./...
if %ERRORLEVEL% neq 0 (
    echo [FAIL] go vet found issues
    exit /b %ERRORLEVEL%
)

echo [BUILD] Running unit tests and route checks...
go test ./...
if %ERRORLEVEL% neq 0 (
    echo [FAIL] Unit tests failed
    exit /b %ERRORLEVEL%
)

echo [BUILD] Checking locale JSON files...
node -e "['ui/locales/en.json','ui/locales/ru.json','ui/locales/tm.json'].forEach(f=>{try{JSON.parse(require('fs').readFileSync(f,'utf8'));console.log('OK',f);}catch(e){console.error('FAIL',f,e.message);process.exit(1);}})"
if %ERRORLEVEL% neq 0 (
    echo [FAIL] JSON locale validation failed
    exit /b 1
)

echo [BUILD] Checking for forbidden Cyrillic text in codebase...
node -e "const fs=require('fs'),path=require('path');function check(d){for(const e of fs.readdirSync(d,{withFileTypes:true})){if(e.name.startsWith('.')||e.name==='node_modules'||e.name==='!Backup'||e.name==='plans')continue;const p=path.join(d,e.name);if(e.isDirectory()){check(p);}else if(/\.(go|js|html|css)$/.test(e.name)){const c=fs.readFileSync(p,'utf8');if(/[\u0400-\u04FF]/.test(c)){console.error('FAIL: Cyrillic text found in '+p);process.exit(1);}}}}check('.');console.log('OK: Codebase is 100%% English.');"
if %ERRORLEVEL% neq 0 (
    echo [FAIL] Cyrillic text detected in codebase
    exit /b 1
)

echo [BUILD] Checking JavaScript modules syntax...
node --check ui\js\state.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] state.js & exit /b 1 )
node --check ui\js\api.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] api.js & exit /b 1 )
node --check ui\js\ws.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] ws.js & exit /b 1 )
node --check ui\js\i18n.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] i18n.js & exit /b 1 )
node --check ui\js\toast.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] toast.js & exit /b 1 )
node --check ui\js\components\modal.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] modal.js & exit /b 1 )
node --check ui\js\components\issue.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] issue.js & exit /b 1 )
node --check ui\js\components\comment.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] comment.js & exit /b 1 )
node --check ui\js\components\card.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] card.js & exit /b 1 )
node --check ui\js\views\plans.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] views/plans.js & exit /b 1 )
node --check ui\js\views\plan-detail.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] views/plan-detail.js & exit /b 1 )
node --check ui\js\main.js
if %ERRORLEVEL% neq 0 ( echo [FAIL] main.js & exit /b 1 )

echo [BUILD] Building binary...
go build -o codraft.exe .
if %ERRORLEVEL% neq 0 (
    echo [FAIL] Build failed
    exit /b %ERRORLEVEL%
)
if exist codraft.exe del /q codraft.exe

echo [OK] ALL BUILDS PASSED SUCCESSFULLY
