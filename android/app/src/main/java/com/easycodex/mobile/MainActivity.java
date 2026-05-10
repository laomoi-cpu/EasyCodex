package com.easycodex.mobile;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.util.Base64;
import android.view.Gravity;
import android.view.View;
import android.view.WindowManager;
import android.view.inputmethod.InputMethodManager;
import android.widget.Button;
import android.widget.EditText;
import android.widget.HorizontalScrollView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends Activity {
    private final ExecutorService executor = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private final List<PaneInfo> panes = new ArrayList<>();

    private EditText baseUrlInput;
    private EditText tokenInput;
    private TextView statusView;
    private LinearLayout panesView;
    private TextView terminalView;
    private ScrollView terminalScroll;
    private EditText commandInput;
    private TextView lastSentView;
    private Button currentPaneButton;
    private LinearLayout keyPanel;
    private boolean keyPanelExpanded = false;

    private String baseUrl = "http://127.0.0.1:8765";
    private String token = "easycodex-dev-token";
    private String instanceId = "main";
    private String defaultInstanceId = "main";
    private String defaultCwd = "D:\\mgame";
    private final List<String> defaultCommand = new ArrayList<>();
    private String paneId = "";
    private String snapshotHash = "";
    private String lastSnapshotText = "";
    private boolean polling = false;
    private int pollToken = 0;
    private int pollFailureCount = 0;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        loadSettings();
        ensureDefaultCommand();
        buildUi();
        handlePairingIntent(getIntent());
        handleAutomationIntent(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handlePairingIntent(intent);
        handleAutomationIntent(intent);
    }

    @Override
    protected void onDestroy() {
        polling = false;
        executor.shutdownNow();
        super.onDestroy();
    }

    private void buildUi() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(dp(8), dp(8), dp(8), dp(8));
        root.setBackgroundColor(0xFFEEF2F7);

        baseUrlInput = input("Agent URL", baseUrl);
        tokenInput = input("Token", token);

        LinearLayout topBar = new LinearLayout(this);
        topBar.setOrientation(LinearLayout.HORIZONTAL);
        topBar.setGravity(Gravity.CENTER_VERTICAL);
        topBar.setPadding(0, 0, 0, dp(6));

        Button statusButton = button("Offline");
        statusView = statusButton;
        styleStatus("Idle");
        Button settingsButton = iconButton("⚙");
        topBar.addView(statusButton, new LinearLayout.LayoutParams(0, dp(38), 1));
        topBar.addView(settingsButton, new LinearLayout.LayoutParams(dp(44), dp(38)));
        root.addView(topBar, matchWrap());

        panesView = new LinearLayout(this);
        panesView.setOrientation(LinearLayout.HORIZONTAL);
        panesView.setGravity(Gravity.CENTER_VERTICAL);
        HorizontalScrollView paneScroll = new HorizontalScrollView(this);
        paneScroll.addView(panesView);

        LinearLayout sessionStrip = new LinearLayout(this);
        sessionStrip.setOrientation(LinearLayout.HORIZONTAL);
        sessionStrip.setGravity(Gravity.CENTER_VERTICAL);
        sessionStrip.setPadding(0, 0, 0, dp(6));
        Button newCodexButton = iconButton("+");
        sessionStrip.addView(newCodexButton, new LinearLayout.LayoutParams(dp(44), dp(42)));
        sessionStrip.addView(paneScroll, new LinearLayout.LayoutParams(0, dp(42), 1));
        root.addView(sessionStrip, matchWrap());

        terminalScroll = new ScrollView(this);
        terminalView = new TextView(this);
        terminalView.setTextColor(0xFFE6EDF3);
        terminalView.setBackground(rounded(0xFF0B1220, dp(8), 0));
        terminalView.setTypeface(Typeface.MONOSPACE);
        terminalView.setTextSize(12.5f);
        terminalView.setPadding(dp(10), dp(10), dp(10), dp(10));
        terminalView.setText("Tap the status button to connect, then select a pane.");
        terminalScroll.addView(terminalView, matchWrap());
        terminalScroll.setFillViewport(true);
        root.addView(terminalScroll, new LinearLayout.LayoutParams(-1, 0, 1));

        lastSentView = smallLabel("");
        root.addView(lastSentView, fixedHeight(dp(18)));

        keyPanel = new LinearLayout(this);
        keyPanel.setOrientation(LinearLayout.HORIZONTAL);
        keyPanel.setGravity(Gravity.CENTER_VERTICAL);
        keyPanel.setVisibility(View.GONE);
        Button enterOnly = compactButton("Enter");
        Button ctrlC = compactButton("Ctrl+C");
        Button tab = compactButton("Tab");
        Button esc = compactButton("Esc");
        Button clearText = compactButton("Clear");
        keyPanel.addView(enterOnly, weightWrap(1));
        keyPanel.addView(ctrlC, weightWrap(1));
        keyPanel.addView(tab, weightWrap(1));
        keyPanel.addView(esc, weightWrap(1));
        keyPanel.addView(clearText, weightWrap(1));
        root.addView(keyPanel, fixedHeight(dp(38)));

        LinearLayout inputRow = new LinearLayout(this);
        inputRow.setOrientation(LinearLayout.HORIZONTAL);
        inputRow.setGravity(Gravity.CENTER_VERTICAL);
        commandInput = multilineInput("Message", "");
        Button moreKeys = iconButton("⌘");
        Button sendEnter = button("Send");
        inputRow.addView(commandInput, new LinearLayout.LayoutParams(0, dp(48), 1));
        inputRow.addView(moreKeys, new LinearLayout.LayoutParams(dp(44), dp(48)));
        inputRow.addView(sendEnter, new LinearLayout.LayoutParams(dp(76), dp(48)));
        root.addView(inputRow, matchWrap());

        setContentView(root);

        statusButton.setOnClickListener(v -> connect());
        settingsButton.setOnClickListener(v -> showSettingsDialog());
        newCodexButton.setOnClickListener(v -> spawnCodexSession());
        sendEnter.setOnClickListener(v -> sendCommand(true));
        clearText.setOnClickListener(v -> commandInput.setText(""));
        moreKeys.setOnClickListener(v -> toggleKeyPanel());
        enterOnly.setOnClickListener(v -> sendRaw("", true));
        ctrlC.setOnClickListener(v -> sendRaw("\u0003", false));
        tab.setOnClickListener(v -> sendRaw("\t", false));
        esc.setOnClickListener(v -> sendRaw("\u001B", false));
    }

    private void connect() {
        saveConnection();
        setStatus("Connecting...");
        request("GET", "/api/health", null, false, result -> {
            if (!result.ok) {
                setStatus("Health failed: " + result.error);
                return;
            }
            loadRemoteConfig();
        });
    }

    private void saveConnection() {
        baseUrl = trimTrailingSlash(baseUrlInput.getText().toString().trim());
        token = tokenInput.getText().toString().trim();
        saveSettings();
        setStatus("Saved");
    }

    private void showSettingsDialog() {
        LinearLayout form = new LinearLayout(this);
        form.setOrientation(LinearLayout.VERTICAL);
        form.setPadding(dp(8), dp(6), dp(8), 0);

        EditText urlField = input("Agent URL", baseUrl);
        EditText tokenField = input("Token", token);
        form.addView(urlField, matchWrap());
        form.addView(tokenField, matchWrap());

        new AlertDialog.Builder(this)
                .setTitle("Server Settings")
                .setView(form)
                .setPositiveButton("Save", (dialog, which) -> {
                    baseUrlInput.setText(urlField.getText().toString());
                    tokenInput.setText(tokenField.getText().toString());
                    saveConnection();
                })
                .setNeutralButton("Connect", (dialog, which) -> {
                    baseUrlInput.setText(urlField.getText().toString());
                    tokenInput.setText(tokenField.getText().toString());
                    connect();
                })
                .setNegativeButton("Cancel", null)
                .show();
    }

    private void toggleKeyPanel() {
        keyPanelExpanded = !keyPanelExpanded;
        keyPanel.setVisibility(keyPanelExpanded ? View.VISIBLE : View.GONE);
    }

    private void loadRemoteConfig() {
        request("GET", "/api/config", null, true, result -> {
            if (!result.ok) {
                setStatus("Config unavailable, using local defaults: " + result.error);
                loadInstances();
                return;
            }
            JSONObject defaults = result.data.optJSONObject("defaults");
            if (defaults != null) {
                defaultInstanceId = defaults.optString("instanceId", defaultInstanceId);
                defaultCwd = defaults.optString("cwd", defaultCwd);
                JSONArray command = defaults.optJSONArray("command");
                if (command != null && command.length() > 0) {
                    defaultCommand.clear();
                    for (int i = 0; i < command.length(); i++) {
                        String part = command.optString(i);
                        if (!part.isEmpty()) {
                            defaultCommand.add(part);
                        }
                    }
                }
                if (!defaultInstanceId.isEmpty()) {
                    instanceId = defaultInstanceId;
                }
            }
            setStatus("Configured " + instanceId);
            loadInstances();
        });
    }

    private void loadInstances() {
        request("GET", "/api/instances", null, true, result -> {
            if (!result.ok) {
                setStatus("Instances failed: " + result.error);
                return;
            }
            JSONArray items = result.data.optJSONArray("instances");
            boolean foundMain = false;
            if (items != null) {
                for (int i = 0; i < items.length(); i++) {
                    JSONObject item = items.optJSONObject(i);
                    if (item != null && instanceId.equals(item.optString("id"))) {
                        foundMain = true;
                        break;
                    }
                }
            }
            if (!foundMain) {
                setStatus("Connected, but instance 'main' was not found.");
                return;
            }
            setStatus("Connected");
            hideKeyboard();
            loadSessions();
        });
    }

    private void loadSessions() {
        request("GET", "/api/instances/" + instanceId + "/sessions", null, true, result -> {
            if (!result.ok) {
                setStatus("Sessions failed: " + result.error);
                return;
            }
            updatePanes(result.data);
            renderPanes();
            PaneInfo current = findPane(paneId);
            if (current != null) {
                setStatus("Panes: " + panes.size() + " | current " + paneId);
            } else if (!panes.isEmpty()) {
                selectPane(panes.get(0).id);
            } else {
                stopPolling();
                paneId = "";
                snapshotHash = "";
                lastSnapshotText = "";
                terminalView.setText("No panes available. Start or refresh the Agent session.");
                setStatus("Panes: 0");
            }
        });
    }

    private void updatePanes(JSONObject data) {
        panes.clear();
        JSONArray items = data.optJSONArray("panes");
        if (items != null) {
            for (int i = 0; i < items.length(); i++) {
                JSONObject item = items.optJSONObject(i);
                if (item == null) {
                    continue;
                }
                PaneInfo pane = new PaneInfo();
                pane.id = item.optString("paneId");
                pane.title = item.optString("title");
                pane.cwd = item.optString("cwd");
                pane.active = item.optBoolean("isActive");
                panes.add(pane);
            }
        }
    }
    private void renderPanes() {
        panesView.removeAllViews();
        currentPaneButton = null;
        for (PaneInfo pane : panes) {
            Button item = button(paneLabel(pane));
            item.setAllCaps(false);
            stylePaneButton(item, pane.id.equals(paneId), pane.active);
            item.setOnClickListener(v -> selectPane(pane.id));
            panesView.addView(item, new LinearLayout.LayoutParams(dp(132), dp(38)));
            if (pane.id.equals(paneId)) {
                currentPaneButton = item;
            }
        }
    }

    private void selectPane(String id) {
        paneId = id;
        snapshotHash = "";
        lastSnapshotText = "";
        pollFailureCount = 0;
        terminalView.setText("Loading pane " + paneId + "...");
        polling = true;
        pollToken++;
        renderPanes();
        setStatus("Pane " + paneId + " selected");
        pollSnapshot(pollToken);
    }

    private void pollSnapshot(int token) {
        if (!polling || paneId.isEmpty()) {
            return;
        }
        if (token != pollToken) {
            return;
        }
        String path = "/api/instances/" + instanceId + "/panes/" + paneId + "/snapshot?lines=160";
        if (!snapshotHash.isEmpty()) {
            path += "&since=" + snapshotHash;
        }
        request("GET", path, null, true, result -> {
            if (token != pollToken) {
                return;
            }
            if (!result.ok) {
                pollFailureCount++;
                setStatus("Snapshot failed: " + result.error);
                schedulePoll(token);
                return;
            }
            pollFailureCount = 0;
            snapshotHash = result.data.optString("hash", snapshotHash);
            if (result.data.optBoolean("changed")) {
                lastSnapshotText = result.data.optString("text");
                terminalView.setText(lastSnapshotText);
                main.postDelayed(() -> terminalScroll.fullScroll(View.FOCUS_DOWN), 40);
            }
            schedulePoll(token);
        });
    }

    private void schedulePoll(int token) {
        int delay = pollFailureCount == 0 ? 1000 : Math.min(5000, 1000 + pollFailureCount * 1000);
        main.postDelayed(() -> pollSnapshot(token), delay);
    }

    private void spawnCodexSession() {
        saveConnection();
        setStatus("Starting Codex @ " + defaultCwd + "...");
        try {
            JSONObject body = new JSONObject();
            JSONArray command = new JSONArray();
            ensureDefaultCommand();
            for (String part : defaultCommand) {
                command.put(part);
            }
            body.put("cwd", defaultCwd);
            body.put("command", command);
            request("POST", "/api/instances/" + instanceId + "/spawn", body, true, result -> {
                if (!result.ok) {
                    setStatus("New Codex failed: " + result.error);
                    return;
                }
                String newPaneId = result.data.optString("paneId");
                setStatus("Started Codex pane " + newPaneId);
                loadSessionsThenSelect(newPaneId);
            });
        } catch (Exception ex) {
            setStatus("New Codex failed: " + ex.getMessage());
        }
    }

    private void loadSessionsThenSelect(String targetPaneId) {
        request("GET", "/api/instances/" + instanceId + "/sessions", null, true, result -> {
            if (!result.ok) {
                setStatus("Sessions failed: " + result.error);
                return;
            }
            updatePanes(result.data);
            renderPanes();
            if (findPane(targetPaneId) != null) {
                selectPane(targetPaneId);
            } else if (!panes.isEmpty()) {
                selectPane(panes.get(0).id);
            } else {
                setStatus("Started Codex, but no panes are visible yet.");
            }
        });
    }
    private void sendCommand(boolean enter) {
        String text = commandInput.getText().toString();
        if (text.isEmpty() && !enter) {
            return;
        }
        sendRaw(text, enter);
        if (enter) {
            commandInput.setText("");
            hideKeyboard(commandInput);
        }
    }

    private void sendRaw(String text, boolean enter) {
        if (paneId.isEmpty()) {
            setStatus("Select a pane first.");
            return;
        }
        try {
            JSONObject body = new JSONObject();
            if (!text.isEmpty()) {
                String encoded = Base64.encodeToString(text.getBytes(StandardCharsets.UTF_8), Base64.NO_WRAP);
                body.put("textBase64", encoded);
            }
            body.put("noPaste", true);
            body.put("enter", enter);
            setStatus("Sending to pane " + paneId + "...");
            request("POST", "/api/instances/" + instanceId + "/panes/" + paneId + "/send", body, true, result -> {
                if (!result.ok) {
                    setStatus("Send failed: " + result.error);
                    return;
                }
                setStatus("Sent to pane " + paneId);
                updateLastSent(text, enter);
                snapshotHash = "";
                pollSnapshot(pollToken);
            });
        } catch (Exception ex) {
            setStatus("Send failed: " + ex.getMessage());
        }
    }

    private void handlePairingIntent(Intent intent) {
        if (intent == null || intent.getData() == null) {
            return;
        }
        Uri uri = intent.getData();
        if (!"easycodex".equals(uri.getScheme()) || !"pair".equals(uri.getHost())) {
            return;
        }
        String pairUrl = uri.getQueryParameter("url");
        if (pairUrl == null || pairUrl.isEmpty()) {
            pairUrl = uri.getQueryParameter("u");
        }
        if (pairUrl != null && !pairUrl.isEmpty()) {
            setStatus("Pairing from PC...");
            requestAbsolute(pairUrl, result -> {
                if (!result.ok) {
                    setStatus("Pairing failed: " + result.error);
                    return;
                }
                applyPairingPayload(result.data);
                connect();
            });
            return;
        }
        String data = uri.getQueryParameter("data");
        if (data == null || data.isEmpty()) {
            setStatus("Pairing failed: missing data");
            return;
        }
        try {
            String json = new String(Base64.decode(data, Base64.DEFAULT), StandardCharsets.UTF_8);
            applyPairingPayload(new JSONObject(json));
            connect();
        } catch (Exception ex) {
            setStatus("Pairing failed: " + ex.getMessage());
        }
    }

    private void applyPairingPayload(JSONObject payload) {
        baseUrl = trimTrailingSlash(payload.optString("baseUrl", baseUrl));
        token = payload.optString("token", token);
        JSONObject defaults = payload.optJSONObject("defaults");
        if (defaults != null) {
            defaultInstanceId = defaults.optString("instanceId", defaultInstanceId);
            defaultCwd = defaults.optString("cwd", defaultCwd);
            JSONArray command = defaults.optJSONArray("command");
            if (command != null && command.length() > 0) {
                defaultCommand.clear();
                for (int i = 0; i < command.length(); i++) {
                    String part = command.optString(i);
                    if (!part.isEmpty()) {
                        defaultCommand.add(part);
                    }
                }
            }
            if (!defaultInstanceId.isEmpty()) {
                instanceId = defaultInstanceId;
            }
        }
        baseUrlInput.setText(baseUrl);
        tokenInput.setText(token);
        saveSettings();
        setStatus("Paired " + baseUrl);
    }
    private void handleAutomationIntent(Intent intent) {
        if (intent == null || (!intent.hasExtra("prompt") && !intent.hasExtra("promptBase64"))) {
            return;
        }
        String prompt = intent.getStringExtra("prompt");
        String promptBase64 = intent.getStringExtra("promptBase64");
        if (promptBase64 != null && !promptBase64.isEmpty()) {
            try {
                prompt = new String(Base64.decode(promptBase64, Base64.DEFAULT), StandardCharsets.UTF_8);
            } catch (Exception ex) {
                setStatus("Automation prompt failed: " + ex.getMessage());
                return;
            }
        }
        boolean enter = intent.getBooleanExtra("sendEnter", true);
        commandInput.setText(prompt == null ? "" : prompt);
        commandInput.setSelection(commandInput.getText().length());
        main.postDelayed(() -> sendCommand(enter), 400);
    }

    private void requestAbsolute(String urlText, Callback callback) {
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
    private void request(String method, String path, JSONObject body, boolean auth, Callback callback) {
        executor.execute(() -> {
            Result result = new Result();
            HttpURLConnection conn = null;
            try {
                URL url = new URL(baseUrl + path);
                conn = (HttpURLConnection) url.openConnection();
                conn.setRequestMethod(method);
                conn.setConnectTimeout(5000);
                conn.setReadTimeout(10000);
                conn.setRequestProperty("Accept", "application/json");
                if (auth) {
                    conn.setRequestProperty("Authorization", "Bearer " + token);
                }
                if (body != null) {
                    byte[] bytes = body.toString().getBytes(StandardCharsets.UTF_8);
                    conn.setDoOutput(true);
                    conn.setRequestProperty("Content-Type", "application/json; charset=utf-8");
                    conn.setRequestProperty("Content-Length", String.valueOf(bytes.length));
                    try (OutputStream out = conn.getOutputStream()) {
                        out.write(bytes);
                    }
                }
                int code = conn.getResponseCode();
                String text = readAll(code >= 400 ? conn.getErrorStream() : conn.getInputStream());
                JSONObject json = new JSONObject(text);
                result.ok = json.optBoolean("ok");
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

    private void ensureDefaultCommand() {
        if (!defaultCommand.isEmpty()) {
            return;
        }
        defaultCommand.add("cmd.exe");
        defaultCommand.add("/k");
        defaultCommand.add("cd /d D:\\mgame && codex --dangerously-bypass-approvals-and-sandbox");
    }

    private void loadSettings() {
        SharedPreferences prefs = getSharedPreferences("easycodex", MODE_PRIVATE);
        baseUrl = prefs.getString("baseUrl", baseUrl);
        token = prefs.getString("token", token);
    }

    private void saveSettings() {
        getSharedPreferences("easycodex", MODE_PRIVATE)
                .edit()
                .putString("baseUrl", baseUrl)
                .putString("token", token)
                .apply();
    }

    private EditText input(String hint, String value) {
        EditText input = new EditText(this);
        input.setHint(hint);
        input.setText(value);
        input.setSingleLine(true);
        input.setTextSize(14);
        input.setTextColor(0xFF111827);
        input.setHintTextColor(0xFF8A94A6);
        input.setBackground(rounded(0xFFFFFFFF, dp(8), 0xFFD5DAE3));
        input.setPadding(dp(10), 0, dp(10), 0);
        return input;
    }

    private EditText multilineInput(String hint, String value) {
        EditText input = input(hint, value);
        input.setSingleLine(false);
        input.setMinLines(1);
        input.setMaxLines(3);
        input.setGravity(Gravity.TOP | Gravity.START);
        input.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_MULTI_LINE | InputType.TYPE_TEXT_FLAG_CAP_SENTENCES);
        input.setPadding(dp(8), dp(4), dp(8), dp(4));
        return input;
    }

    private TextView label(String text) {
        TextView view = new TextView(this);
        view.setText(text);
        view.setTextColor(0xFF374151);
        view.setTextSize(13);
        view.setPadding(0, dp(4), 0, dp(4));
        return view;
    }

    private TextView smallLabel(String text) {
        TextView view = new TextView(this);
        view.setText(text);
        view.setTextColor(0xFF6B7280);
        view.setTextSize(10);
        view.setSingleLine(true);
        view.setPadding(dp(2), dp(2), 0, 0);
        return view;
    }

    private Button button(String text) {
        Button button = new Button(this);
        button.setText(text);
        button.setAllCaps(false);
        button.setTextSize(13);
        button.setSingleLine(true);
        button.setTextColor(0xFFFFFFFF);
        button.setBackground(rounded(0xFF2563EB, dp(8), 0));
        button.setMinHeight(0);
        button.setMinimumHeight(0);
        button.setPadding(dp(8), 0, dp(8), 0);
        return button;
    }

    private Button iconButton(String text) {
        Button button = button(text);
        button.setTextSize(18);
        button.setTextColor(0xFF1F2937);
        button.setBackground(rounded(0xFFFFFFFF, dp(8), 0xFFD5DAE3));
        return button;
    }

    private Button compactButton(String text) {
        Button button = button(text);
        button.setTextSize(11);
        button.setTextColor(0xFF1F2937);
        button.setBackground(rounded(0xFFE8EEF8, dp(7), 0xFFD5DAE3));
        return button;
    }

    private void stylePaneButton(Button button, boolean selected, boolean active) {
        button.setTextColor(selected ? 0xFFFFFFFF : 0xFF111827);
        if (selected) {
            button.setBackground(rounded(0xFF2563EB, dp(8), 0));
        } else if (active) {
            button.setBackground(rounded(0xFFD1FAE5, dp(8), 0xFFA7F3D0));
        } else {
            button.setBackground(rounded(0xFFFFFFFF, dp(8), 0xFFD5DAE3));
        }
        button.setTextSize(11);
        button.setPadding(dp(8), 0, dp(8), 0);
    }

    private void setStatus(String text) {
        if (statusView == null) {
            return;
        }
        statusView.setText(text);
        styleStatus(text);
    }

    private void styleStatus(String text) {
        if (statusView == null) {
            return;
        }
        String value = text == null ? "" : text.toLowerCase();
        int fill = 0xFFE5E7EB;
        int stroke = 0xFFD1D5DB;
        int textColor = 0xFF374151;
        if (value.contains("connected") || value.contains("configured") || value.contains("sent") || value.contains("started") || value.contains("saved")) {
            fill = 0xFFD1FAE5;
            stroke = 0xFF86EFAC;
            textColor = 0xFF065F46;
        } else if (value.contains("connecting") || value.contains("loading") || value.contains("sending") || value.contains("starting") || value.contains("pane ")) {
            fill = 0xFFDBEAFE;
            stroke = 0xFF93C5FD;
            textColor = 0xFF1D4ED8;
        } else if (value.contains("failed") || value.contains("error") || value.contains("unavailable") || value.contains("select")) {
            fill = 0xFFFEE2E2;
            stroke = 0xFFFCA5A5;
            textColor = 0xFF991B1B;
        }
        statusView.setTextColor(textColor);
        statusView.setBackground(rounded(fill, dp(999), stroke));
    }

    private GradientDrawable rounded(int fill, int radius, int stroke) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(fill);
        drawable.setCornerRadius(radius);
        if (stroke != 0) {
            drawable.setStroke(dp(1), stroke);
        }
        return drawable;
    }

    private void hideKeyboard() {
        hideKeyboard(baseUrlInput);
    }

    private void hideKeyboard(View view) {
        InputMethodManager imm = (InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE);
        if (imm != null && view != null) {
            imm.hideSoftInputFromWindow(view.getWindowToken(), 0);
        }
    }

    private void stopPolling() {
        polling = false;
        pollToken++;
    }

    private PaneInfo findPane(String id) {
        if (id == null || id.isEmpty()) {
            return null;
        }
        for (PaneInfo pane : panes) {
            if (id.equals(pane.id)) {
                return pane;
            }
        }
        return null;
    }

    private String paneLabel(PaneInfo pane) {
        String prefix = pane.active ? "* " : "";
        String label = prefix + pane.id + " " + safeTitle(pane);
        if (label.length() > 24) {
            label = label.substring(0, 23) + "...";
        }
        return label;
    }

    private String safeTitle(PaneInfo pane) {
        if (pane.title != null && !pane.title.isEmpty()) {
            return pane.title;
        }
        return pane.cwd == null ? "" : pane.cwd;
    }

    private void updateLastSent(String text, boolean enter) {
        if (lastSentView == null) {
            return;
        }
        String value = text == null || text.isEmpty() ? (enter ? "<Enter>" : "<empty>") : text;
        value = value.replace("\r", "\\r").replace("\n", "\\n");
        if (value.length() > 96) {
            value = value.substring(0, 96) + "...";
        }
        lastSentView.setText("Last sent: " + value + (enter ? " + Enter" : ""));
    }

    private String trimTrailingSlash(String value) {
        while (value.endsWith("/")) {
            value = value.substring(0, value.length() - 1);
        }
        return value;
    }

    private LinearLayout.LayoutParams matchWrap() {
        return new LinearLayout.LayoutParams(-1, -2);
    }

    private LinearLayout.LayoutParams fixedHeight(int height) {
        return new LinearLayout.LayoutParams(-1, height);
    }

    private LinearLayout.LayoutParams weightWrap(float weight) {
        return new LinearLayout.LayoutParams(0, -2, weight);
    }

    private int dp(int value) {
        return (int) (value * getResources().getDisplayMetrics().density + 0.5f);
    }

    private interface Callback {
        void done(Result result);
    }

    private static class Result {
        boolean ok;
        JSONObject data;
        String error;
    }

    private static class PaneInfo {
        String id;
        String title;
        String cwd;
        boolean active;
    }
}
