package com.easycodex.mobile;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.util.Base64;
import android.view.Gravity;
import android.view.View;
import android.view.WindowManager;
import android.view.inputmethod.InputMethodManager;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.AdapterView;
import android.widget.ArrayAdapter;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.Spinner;
import android.widget.TextView;

import com.google.zxing.integration.android.IntentIntegrator;
import com.google.zxing.integration.android.IntentResult;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Date;
import java.util.List;
import java.util.Locale;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends Activity {
    private static final int REQUEST_CAMERA_SCAN = 41;
    private static final int REQUEST_FILE_CHOOSER = 42;
    private static final int MAX_CONNECTION_HISTORY = 10;

    private final ExecutorService executor = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private final List<ConnectionHistoryItem> connectionHistory = new ArrayList<>();

    private WebView webView;
    private TextView statusView;
    private ValueCallback<Uri[]> fileChooserCallback;

    private String baseUrl = "http://127.0.0.1:8765";
    private String token = "";
    private String clientId = "";

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_PORTRAIT);
        enterImmersiveFullscreen();
        getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        loadSettings();
        buildUi();
        handlePairingIntent(getIntent());
        if (token.isEmpty()) {
            showSettingsDialog();
        } else {
            loadTerminal();
            testConnection(false);
        }
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handlePairingIntent(intent);
    }

    @Override
    protected void onResume() {
        super.onResume();
        enterImmersiveFullscreen();
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (hasFocus) {
            enterImmersiveFullscreen();
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        if (requestCode == REQUEST_FILE_CHOOSER) {
            handleFileChooserResult(resultCode, data);
            return;
        }
        IntentResult result = IntentIntegrator.parseActivityResult(requestCode, resultCode, data);
        if (result != null) {
            String contents = result.getContents();
            if (contents == null || contents.trim().isEmpty()) {
                setStatus("取消扫码", "warn");
            } else {
                handleScannedPairing(contents.trim());
            }
            return;
        }
        super.onActivityResult(requestCode, resultCode, data);
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == REQUEST_CAMERA_SCAN) {
            if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
                launchQrScanner();
            } else {
                setStatus("相机权限被拒绝", "err");
            }
        }
    }

    @Override
    public void onBackPressed() {
        if (webView != null && webView.canGoBack()) {
            webView.goBack();
            return;
        }
        super.onBackPressed();
    }

    @Override
    protected void onDestroy() {
        if (webView != null) {
            webView.destroy();
        }
        executor.shutdownNow();
        super.onDestroy();
    }

    private void handleFileChooserResult(int resultCode, Intent data) {
        if (fileChooserCallback == null) {
            return;
        }
        Uri[] result = null;
        if (resultCode == RESULT_OK && data != null) {
            ClipData clipData = data.getClipData();
            if (clipData != null && clipData.getItemCount() > 0) {
                result = new Uri[clipData.getItemCount()];
                for (int i = 0; i < clipData.getItemCount(); i++) {
                    result[i] = clipData.getItemAt(i).getUri();
                }
            } else if (data.getData() != null) {
                result = new Uri[]{data.getData()};
            }
        }
        fileChooserCallback.onReceiveValue(result);
        fileChooserCallback = null;
    }

    private void buildUi() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(0xFF000000);

        LinearLayout topBar = new LinearLayout(this);
        topBar.setOrientation(LinearLayout.HORIZONTAL);
        topBar.setGravity(Gravity.CENTER_VERTICAL);
        topBar.setPadding(dp(8), dp(6), dp(8), dp(6));
        topBar.setBackgroundColor(0xFF111827);

        statusView = new TextView(this);
        statusView.setTextSize(12);
        statusView.setTypeface(Typeface.DEFAULT_BOLD);
        statusView.setSingleLine(true);
        statusView.setGravity(Gravity.CENTER_VERTICAL);
        setStatus("Offline", "warn");

        Button scanButton = iconButton("扫码");
        Button settingsButton = iconButton("设置");
        topBar.addView(statusView, rowWeightParams(1, dp(34), 0, dp(6)));
        topBar.addView(scanButton, rowFixedParams(dp(56), dp(34), 0, dp(6)));
        topBar.addView(settingsButton, rowFixedParams(dp(56), dp(34), 0, 0));
        root.addView(topBar, matchWrap());

        webView = new WebView(this);
        configureWebView();
        root.addView(webView, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(root);

        statusView.setOnClickListener(v -> testConnection(true));
        scanButton.setOnClickListener(v -> startQrScan());
        settingsButton.setOnClickListener(v -> showSettingsDialog());
    }

    private void configureWebView() {
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setLoadWithOverviewMode(false);
        settings.setUseWideViewPort(false);
        settings.setBuiltInZoomControls(false);
        settings.setDisplayZoomControls(false);
        settings.setMediaPlaybackRequiresUserGesture(false);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            settings.setMixedContentMode(WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE);
        }
        webView.setBackgroundColor(Color.BLACK);
        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> filePathCallback, FileChooserParams fileChooserParams) {
                if (fileChooserCallback != null) {
                    fileChooserCallback.onReceiveValue(null);
                }
                fileChooserCallback = filePathCallback;
                showAttachmentChooser(fileChooserParams);
                return true;
            }
        });
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                Uri uri = request.getUrl();
                if (handlePairingUri(uri)) {
                    return true;
                }
                return false;
            }

            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                if (handlePairingUri(Uri.parse(url))) {
                    return true;
                }
                return false;
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                applyAndroidPageChrome();
                setStatus("已打开", "ok");
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M && request.isForMainFrame()) {
                    setStatus("加载失败: " + error.getDescription(), "err");
                    updateConnectionHistoryStatus(baseUrl, "fail");
                }
            }
        });
    }

    private void loadTerminal() {
        saveSettings();
        rememberConnection(baseUrl, token, "saved");
        String url = terminalUrl();
        setStatus("加载中", "work");
        webView.loadUrl(url);
    }

    private void enterImmersiveFullscreen() {
        getWindow().setFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN, WindowManager.LayoutParams.FLAG_FULLSCREEN);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.KITKAT) {
            getWindow().getDecorView().setSystemUiVisibility(
                    View.SYSTEM_UI_FLAG_FULLSCREEN
                            | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                            | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
                            | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                            | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
                            | View.SYSTEM_UI_FLAG_LAYOUT_STABLE);
        }
    }

    private void applyAndroidPageChrome() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.KITKAT || webView == null) {
            return;
        }
        webView.evaluateJavascript("(function(){var s=document.getElementById('easycodex-android-css');if(!s){s=document.createElement('style');s.id='easycodex-android-css';document.head.appendChild(s);}s.textContent='.terminal-statusbar{display:none!important;} .terminal-output{padding-top:10px!important;}';})();", null);
    }

    private void showAttachmentChooser(WebChromeClient.FileChooserParams fileChooserParams) {
        final boolean[] launchingPicker = {false};
        AlertDialog dialog = new AlertDialog.Builder(this)
                .setTitle("添加附件")
                .setMessage("选择文件作为附件，或取消本次添加。")
                .setNegativeButton("取消", (dialogInterface, which) -> cancelFileChooser())
                .setPositiveButton("选择文件", (dialogInterface, which) -> {
                    launchingPicker[0] = true;
                    launchFileChooser(fileChooserParams);
                })
                .create();
        dialog.setOnCancelListener(dialogInterface -> cancelFileChooser());
        dialog.setOnDismissListener(dialogInterface -> {
            if (!launchingPicker[0] && fileChooserCallback != null) {
                cancelFileChooser();
            }
        });
        dialog.show();
    }

    private void launchFileChooser(WebChromeClient.FileChooserParams fileChooserParams) {
        Intent intent = buildFileChooserIntent(fileChooserParams);
        try {
            startActivityForResult(Intent.createChooser(intent, "选择附件"), REQUEST_FILE_CHOOSER);
        } catch (Exception ex) {
            cancelFileChooser();
            setStatus("无法打开文件选择器: " + ex.getMessage(), "err");
        }
    }

    private Intent buildFileChooserIntent(WebChromeClient.FileChooserParams fileChooserParams) {
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType("*/*");
        intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.KITKAT) {
            intent.addFlags(Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP && fileChooserParams != null) {
            String[] acceptTypes = fileChooserParams.getAcceptTypes();
            List<String> filtered = new ArrayList<>();
            if (acceptTypes != null) {
                for (String acceptType : acceptTypes) {
                    if (acceptType != null && !acceptType.trim().isEmpty()) {
                        filtered.add(acceptType.trim());
                    }
                }
            }
            if (filtered.size() == 1) {
                intent.setType(filtered.get(0));
            } else if (filtered.size() > 1) {
                intent.putExtra(Intent.EXTRA_MIME_TYPES, filtered.toArray(new String[0]));
            }
            if (fileChooserParams.getMode() == WebChromeClient.FileChooserParams.MODE_OPEN) {
                intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, false);
            }
        }
        return intent;
    }

    private void cancelFileChooser() {
        if (fileChooserCallback != null) {
            fileChooserCallback.onReceiveValue(null);
            fileChooserCallback = null;
        }
    }

    private String terminalUrl() {
        return baseUrl + "/terminal#baseUrl=" + Uri.encode(baseUrl) + "&token=" + Uri.encode(token);
    }

    private void testConnection(boolean reloadOnSuccess) {
        setStatus("连接中", "work");
        requestAbsolute(baseUrl + "/api/health", true, result -> {
            if (result.ok) {
                updateConnectionHistoryStatus(baseUrl, "ok");
                setStatus("Online", "ok");
                if (reloadOnSuccess) {
                    loadTerminal();
                }
            } else {
                updateConnectionHistoryStatus(baseUrl, "fail");
                setStatus("Offline: " + result.error, "err");
            }
        });
    }

    private void showSettingsDialog() {
        LinearLayout panel = new LinearLayout(this);
        panel.setOrientation(LinearLayout.VERTICAL);
        panel.setPadding(dp(18), dp(14), dp(18), dp(8));

        TextView title = new TextView(this);
        title.setText("服务器连接");
        title.setTextSize(18);
        title.setTypeface(Typeface.DEFAULT_BOLD);
        title.setTextColor(0xFF111827);
        panel.addView(title, matchWrap());

        TextView hint = smallLabel("选择历史连接，手动填写地址，或扫码 PC 配对二维码。");
        hint.setSingleLine(false);
        hint.setPadding(0, dp(4), 0, dp(12));
        panel.addView(hint, matchWrap());

        Spinner historySpinner = new Spinner(this);
        List<String> historyLabels = connectionHistoryLabels();
        ArrayAdapter<String> historyAdapter = new ArrayAdapter<>(this, android.R.layout.simple_spinner_item, historyLabels);
        historyAdapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item);
        historySpinner.setAdapter(historyAdapter);

        EditText urlField = input("Agent URL", baseUrl);
        EditText tokenField = input("Token", token);
        panel.addView(fieldLabel("最近连接"), matchWrap());
        panel.addView(historySpinner, fixedHeight(dp(44)));
        panel.addView(fieldSpacer(), fixedHeight(dp(10)));
        panel.addView(fieldLabel("Agent 地址"), matchWrap());
        panel.addView(urlField, fixedHeight(dp(44)));
        panel.addView(fieldSpacer(), fixedHeight(dp(10)));
        panel.addView(fieldLabel("Token"), matchWrap());
        panel.addView(tokenField, fixedHeight(dp(44)));

        LinearLayout actions = new LinearLayout(this);
        actions.setOrientation(LinearLayout.HORIZONTAL);
        actions.setGravity(Gravity.CENTER_VERTICAL);
        actions.setPadding(0, dp(16), 0, 0);
        Button scanButton = compactButton("扫码");
        Button saveButton = compactButton("保存");
        Button connectButton = button("连接");
        actions.addView(scanButton, rowWeightParams(1, dp(42), 0, dp(6)));
        actions.addView(saveButton, rowWeightParams(1, dp(42), 0, dp(6)));
        actions.addView(connectButton, rowWeightParams(1, dp(42), 0, 0));
        panel.addView(actions, matchWrap());

        AlertDialog dialog = new AlertDialog.Builder(this).create();
        dialog.setView(panel);
        historySpinner.setOnItemSelectedListener(new AdapterView.OnItemSelectedListener() {
            @Override
            public void onItemSelected(AdapterView<?> parent, View view, int position, long id) {
                if (position <= 0 || position > connectionHistory.size()) {
                    return;
                }
                ConnectionHistoryItem item = connectionHistory.get(position - 1);
                urlField.setText(item.baseUrl);
                tokenField.setText(item.token);
            }

            @Override
            public void onNothingSelected(AdapterView<?> parent) {
            }
        });
        scanButton.setOnClickListener(v -> {
            dialog.dismiss();
            startQrScan();
        });
        saveButton.setOnClickListener(v -> {
            applyConnection(urlField, tokenField);
            dialog.dismiss();
        });
        connectButton.setOnClickListener(v -> {
            applyConnection(urlField, tokenField);
            dialog.dismiss();
            loadTerminal();
            testConnection(false);
        });
        dialog.show();
    }

    private void applyConnection(EditText urlField, EditText tokenField) {
        baseUrl = trimTrailingSlash(urlField.getText().toString().trim());
        token = tokenField.getText().toString().trim();
        saveSettings();
        rememberConnection(baseUrl, token, "saved");
        hideKeyboard(urlField);
    }

    private void startQrScan() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M && checkSelfPermission(Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.CAMERA}, REQUEST_CAMERA_SCAN);
            return;
        }
        launchQrScanner();
    }

    private void launchQrScanner() {
        IntentIntegrator integrator = new IntentIntegrator(this);
        integrator.setCaptureActivity(EasyCodexCaptureActivity.class);
        integrator.setDesiredBarcodeFormats(IntentIntegrator.QR_CODE);
        integrator.setPrompt("扫描 EasyCodex 配对二维码");
        integrator.setBeepEnabled(false);
        integrator.setOrientationLocked(false);
        Intent intent = integrator.createScanIntent();
        intent.removeFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP);
        intent.removeFlags(Intent.FLAG_ACTIVITY_CLEAR_WHEN_TASK_RESET);
        startActivityForResult(intent, IntentIntegrator.REQUEST_CODE);
    }

    private void handlePairingIntent(Intent intent) {
        if (intent == null || intent.getData() == null) {
            return;
        }
        handlePairingUri(intent.getData());
    }

    private boolean handlePairingUri(Uri uri) {
        if (uri == null || !"easycodex".equals(uri.getScheme()) || !"pair".equals(uri.getHost())) {
            return false;
        }
        String pairUrl = uri.getQueryParameter("url");
        if (pairUrl == null || pairUrl.isEmpty()) {
            pairUrl = uri.getQueryParameter("u");
        }
        if (pairUrl != null && !pairUrl.isEmpty()) {
            final String finalPairUrl = pairUrl;
            saveScannedBaseUrl(baseUrlFromPairEndpoint(finalPairUrl));
            setStatus("配对中", "work");
            requestAbsolute(finalPairUrl, false, result -> {
                if (!result.ok) {
                    updateConnectionHistoryStatus(baseUrlFromPairEndpoint(finalPairUrl), "fail");
                    setStatus("配对失败: " + result.error, "err");
                    return;
                }
                applyPairingPayload(result.data);
                loadTerminal();
                testConnection(false);
            });
            return true;
        }
        String data = uri.getQueryParameter("data");
        if (data == null || data.isEmpty()) {
            setStatus("配对失败: 缺少数据", "err");
            return true;
        }
        try {
            String json = new String(Base64.decode(data, Base64.DEFAULT), StandardCharsets.UTF_8);
            applyPairingPayload(new JSONObject(json));
            loadTerminal();
            testConnection(false);
        } catch (Exception ex) {
            setStatus("配对失败: " + ex.getMessage(), "err");
        }
        return true;
    }

    private void handleScannedPairing(String contents) {
        try {
            Uri uri = Uri.parse(contents);
            if (handlePairingUri(uri)) {
                return;
            }
            String scheme = uri.getScheme();
            String path = uri.getPath();
            if (("http".equals(scheme) || "https".equals(scheme)) && path != null && path.contains("/api/mobile-pair")) {
                saveScannedBaseUrl(baseUrlFromPairEndpoint(contents));
                setStatus("配对中", "work");
                requestAbsolute(contents, false, result -> {
                    if (!result.ok) {
                        updateConnectionHistoryStatus(baseUrlFromPairEndpoint(contents), "fail");
                        setStatus("配对失败: " + result.error, "err");
                        return;
                    }
                    applyPairingPayload(result.data);
                    loadTerminal();
                    testConnection(false);
                });
                return;
            }
            if ("http".equals(scheme) || "https".equals(scheme)) {
                applyHttpScan(uri);
                loadTerminal();
                testConnection(false);
                return;
            }
            setStatus("不支持的二维码", "err");
        } catch (Exception ex) {
            setStatus("扫码失败: " + ex.getMessage(), "err");
        }
    }

    private void applyHttpScan(Uri uri) {
        String scanned = uri.toString();
        String hash = uri.getFragment();
        if (hash != null && !hash.isEmpty()) {
            Uri fake = Uri.parse("easycodex://hash?" + hash);
            String hashBaseUrl = fake.getQueryParameter("baseUrl");
            String hashToken = fake.getQueryParameter("token");
            if (hashBaseUrl != null && !hashBaseUrl.isEmpty()) {
                baseUrl = trimTrailingSlash(hashBaseUrl);
            } else {
                baseUrl = baseUrlFromPairEndpoint(scanned);
            }
            if (hashToken != null) {
                token = hashToken;
            }
        } else {
            baseUrl = trimTrailingSlash(baseUrlFromPairEndpoint(scanned));
        }
        saveSettings();
        rememberConnection(baseUrl, token, "saved");
        setStatus("已读取二维码", "work");
    }

    private void saveScannedBaseUrl(String scannedBaseUrl) {
        String nextBaseUrl = trimTrailingSlash(scannedBaseUrl == null ? "" : scannedBaseUrl.trim());
        if (nextBaseUrl.isEmpty()) {
            return;
        }
        baseUrl = nextBaseUrl;
        saveSettings();
        rememberConnection(baseUrl, token, "saved");
    }

    private String baseUrlFromPairEndpoint(String endpoint) {
        try {
            Uri uri = Uri.parse(endpoint);
            String scheme = uri.getScheme();
            String authority = uri.getEncodedAuthority();
            if ((scheme == null || scheme.isEmpty()) || (authority == null || authority.isEmpty())) {
                return endpoint;
            }
            return scheme + "://" + authority;
        } catch (Exception ignored) {
            return endpoint;
        }
    }

    private void applyPairingPayload(JSONObject payload) {
        baseUrl = trimTrailingSlash(payload.optString("baseUrl", baseUrl));
        token = payload.optString("token", token);
        saveSettings();
        rememberConnection(baseUrl, token, "ok");
        setStatus("已配对", "ok");
    }

    private void requestAbsolute(String urlText, boolean auth, Callback callback) {
        executor.execute(() -> {
            Result result = new Result();
            HttpURLConnection conn = null;
            try {
                URL url = new URL(urlText);
                conn = (HttpURLConnection) url.openConnection();
                conn.setRequestMethod("GET");
                conn.setConnectTimeout(5000);
                conn.setReadTimeout(10000);
                conn.setRequestProperty("Accept", "application/json");
                applyClientHeaders(conn);
                if (auth && !token.isEmpty()) {
                    conn.setRequestProperty("Authorization", "Bearer " + token);
                }
                int code = conn.getResponseCode();
                String text = readAll(code >= 400 ? conn.getErrorStream() : conn.getInputStream());
                JSONObject json = new JSONObject(text.isEmpty() ? "{}" : text);
                result.ok = code >= 200 && code < 300 && json.optBoolean("ok", true);
                result.data = json.optJSONObject("data");
                if (result.data == null) {
                    result.data = new JSONObject();
                }
                result.error = json.optString("error");
                if (!result.ok && result.error.isEmpty()) {
                    result.error = "HTTP " + code;
                }
            } catch (Exception ex) {
                result.ok = false;
                result.data = new JSONObject();
                result.error = ex.getMessage();
            } finally {
                if (conn != null) {
                    conn.disconnect();
                }
            }
            main.post(() -> callback.done(result));
        });
    }

    private void applyClientHeaders(HttpURLConnection conn) {
        conn.setRequestProperty("User-Agent", "EasyCodex-Android/2");
        conn.setRequestProperty("X-EasyCodex-Client-ID", clientId);
        conn.setRequestProperty("X-EasyCodex-Client-Kind", "android");
        conn.setRequestProperty("X-EasyCodex-Client-Name", "Android App");
    }

    private String readAll(InputStream stream) throws Exception {
        if (stream == null) {
            return "{}";
        }
        StringBuilder builder = new StringBuilder();
        try (BufferedReader reader = new BufferedReader(new InputStreamReader(stream, StandardCharsets.UTF_8))) {
            String line;
            while ((line = reader.readLine()) != null) {
                builder.append(line);
            }
        }
        return builder.toString();
    }

    private void loadSettings() {
        SharedPreferences prefs = getSharedPreferences("easycodex", MODE_PRIVATE);
        baseUrl = prefs.getString("baseUrl", baseUrl);
        token = prefs.getString("token", token);
        clientId = prefs.getString("clientId", "");
        if (clientId.isEmpty()) {
            clientId = "android:" + UUID.randomUUID().toString();
            prefs.edit().putString("clientId", clientId).apply();
        }
        loadConnectionHistory(prefs);
    }

    private void saveSettings() {
        getSharedPreferences("easycodex", MODE_PRIVATE)
                .edit()
                .putString("baseUrl", baseUrl)
                .putString("token", token)
                .apply();
    }

    private void loadConnectionHistory(SharedPreferences prefs) {
        connectionHistory.clear();
        String raw = prefs.getString("connectionHistory", "[]");
        try {
            JSONArray items = new JSONArray(raw);
            for (int i = 0; i < items.length() && connectionHistory.size() < MAX_CONNECTION_HISTORY; i++) {
                JSONObject item = items.optJSONObject(i);
                if (item == null) {
                    continue;
                }
                String url = trimTrailingSlash(item.optString("baseUrl", "").trim());
                String savedToken = item.optString("token", "").trim();
                long lastUsedAt = item.optLong("lastUsedAt", 0);
                String status = item.optString("status", "unknown");
                if (!url.isEmpty()) {
                    connectionHistory.add(new ConnectionHistoryItem(url, savedToken, lastUsedAt, status));
                }
            }
        } catch (Exception ignored) {
            connectionHistory.clear();
        }
    }

    private void rememberConnection(String url, String savedToken, String status) {
        String normalizedUrl = trimTrailingSlash(url == null ? "" : url.trim());
        if (normalizedUrl.isEmpty()) {
            return;
        }
        for (int i = connectionHistory.size() - 1; i >= 0; i--) {
            ConnectionHistoryItem item = connectionHistory.get(i);
            if (item.baseUrl.equals(normalizedUrl)) {
                connectionHistory.remove(i);
            }
        }
        connectionHistory.add(0, new ConnectionHistoryItem(normalizedUrl, savedToken, System.currentTimeMillis(), normalizeConnectionStatus(status)));
        while (connectionHistory.size() > MAX_CONNECTION_HISTORY) {
            connectionHistory.remove(connectionHistory.size() - 1);
        }
        saveConnectionHistory();
    }

    private void updateConnectionHistoryStatus(String url, String status) {
        String normalizedUrl = trimTrailingSlash(url == null ? "" : url.trim());
        if (normalizedUrl.isEmpty()) {
            return;
        }
        for (ConnectionHistoryItem item : connectionHistory) {
            if (item.baseUrl.equals(normalizedUrl)) {
                item.status = normalizeConnectionStatus(status);
                item.lastUsedAt = System.currentTimeMillis();
                item.token = token;
                saveConnectionHistory();
                return;
            }
        }
        rememberConnection(normalizedUrl, token, status);
    }

    private void saveConnectionHistory() {
        JSONArray items = new JSONArray();
        for (ConnectionHistoryItem item : connectionHistory) {
            JSONObject json = new JSONObject();
            try {
                json.put("baseUrl", item.baseUrl);
                json.put("token", item.token);
                json.put("lastUsedAt", item.lastUsedAt);
                json.put("status", normalizeConnectionStatus(item.status));
                items.put(json);
            } catch (Exception ignored) {
            }
        }
        getSharedPreferences("easycodex", MODE_PRIVATE)
                .edit()
                .putString("connectionHistory", items.toString())
                .apply();
    }

    private List<String> connectionHistoryLabels() {
        List<String> labels = new ArrayList<>();
        labels.add(connectionHistory.isEmpty() ? "暂无历史连接" : "选择历史连接");
        SimpleDateFormat format = new SimpleDateFormat("MM-dd HH:mm", Locale.getDefault());
        for (ConnectionHistoryItem item : connectionHistory) {
            String time = item.lastUsedAt > 0 ? format.format(new Date(item.lastUsedAt)) : "unknown";
            labels.add(statusBadge(item.status) + " " + item.baseUrl + "    " + time);
        }
        return labels;
    }

    private String normalizeConnectionStatus(String status) {
        if ("ok".equals(status) || "fail".equals(status) || "saved".equals(status)) {
            return status;
        }
        return "unknown";
    }

    private String statusBadge(String status) {
        String normalized = normalizeConnectionStatus(status);
        if ("ok".equals(normalized)) {
            return "[OK]";
        }
        if ("fail".equals(normalized)) {
            return "[FAIL]";
        }
        if ("saved".equals(normalized)) {
            return "[SAVED]";
        }
        return "[?]";
    }

    private void setStatus(String text, String kind) {
        if (statusView == null) {
            return;
        }
        statusView.setText(text);
        int bg;
        int fg;
        if ("ok".equals(kind)) {
            bg = 0xFFDCFCE7;
            fg = 0xFF166534;
        } else if ("err".equals(kind)) {
            bg = 0xFFFEE2E2;
            fg = 0xFF991B1B;
        } else if ("work".equals(kind)) {
            bg = 0xFFDBEAFE;
            fg = 0xFF1D4ED8;
        } else {
            bg = 0xFFF3F4F6;
            fg = 0xFF374151;
        }
        statusView.setTextColor(fg);
        statusView.setBackground(rounded(bg, dp(8), 0));
        statusView.setPadding(dp(10), 0, dp(10), 0);
    }

    private EditText input(String hint, String value) {
        EditText edit = new EditText(this);
        edit.setSingleLine(true);
        edit.setHint(hint);
        edit.setText(value == null ? "" : value);
        edit.setTextSize(14);
        edit.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        edit.setPadding(dp(10), 0, dp(10), 0);
        edit.setBackground(rounded(0xFFFFFFFF, dp(7), 0xFFD1D5DB));
        return edit;
    }

    private TextView smallLabel(String value) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(12);
        view.setTextColor(0xFF667085);
        return view;
    }

    private TextView fieldLabel(String value) {
        TextView view = smallLabel(value);
        view.setTypeface(Typeface.DEFAULT_BOLD);
        view.setTextColor(0xFF344054);
        return view;
    }

    private View fieldSpacer() {
        return new View(this);
    }

    private Button button(String text) {
        Button button = new Button(this);
        button.setAllCaps(false);
        button.setText(text);
        button.setTextSize(13);
        button.setTypeface(Typeface.DEFAULT_BOLD);
        button.setTextColor(Color.WHITE);
        button.setBackground(rounded(0xFF2563EB, dp(8), 0));
        return button;
    }

    private Button compactButton(String text) {
        Button button = button(text);
        button.setTextSize(12);
        button.setPadding(dp(4), 0, dp(4), 0);
        return button;
    }

    private Button iconButton(String text) {
        Button button = compactButton(text);
        button.setTextColor(0xFF111827);
        button.setBackground(rounded(0xFFFFFFFF, dp(8), 0xFFD1D5DB));
        return button;
    }

    private GradientDrawable rounded(int color, int radius, int strokeColor) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(color);
        drawable.setCornerRadius(radius);
        if (strokeColor != 0) {
            drawable.setStroke(dp(1), strokeColor);
        }
        return drawable;
    }

    private LinearLayout.LayoutParams matchWrap() {
        return new LinearLayout.LayoutParams(-1, -2);
    }

    private LinearLayout.LayoutParams fixedHeight(int height) {
        return new LinearLayout.LayoutParams(-1, height);
    }

    private LinearLayout.LayoutParams rowFixedParams(int width, int height, int left, int right) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(width, height);
        params.setMargins(left, 0, right, 0);
        return params;
    }

    private LinearLayout.LayoutParams rowWeightParams(float weight, int height, int left, int right) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(0, height, weight);
        params.setMargins(left, 0, right, 0);
        return params;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    private String trimTrailingSlash(String value) {
        if (value == null) {
            return "";
        }
        String next = value.trim();
        while (next.endsWith("/") && next.length() > 1) {
            next = next.substring(0, next.length() - 1);
        }
        return next;
    }

    private void hideKeyboard(View view) {
        InputMethodManager manager = (InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE);
        if (manager != null) {
            manager.hideSoftInputFromWindow(view.getWindowToken(), 0);
        }
    }

    private interface Callback {
        void done(Result result);
    }

    private static class Result {
        boolean ok;
        JSONObject data = new JSONObject();
        String error = "";
    }

    private static class ConnectionHistoryItem {
        String baseUrl;
        String token;
        long lastUsedAt;
        String status;

        ConnectionHistoryItem(String baseUrl, String token, long lastUsedAt, String status) {
            this.baseUrl = baseUrl;
            this.token = token;
            this.lastUsedAt = lastUsedAt;
            this.status = status;
        }
    }
}
