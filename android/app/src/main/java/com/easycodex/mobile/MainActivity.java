package com.easycodex.mobile;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Typeface;
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

    private String baseUrl = "http://127.0.0.1:8765";
    private String token = "easycodex-dev-token";
    private String instanceId = "main";
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
        buildUi();
        handleAutomationIntent(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
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
        root.setPadding(dp(12), dp(10), dp(12), dp(10));
        root.setBackgroundColor(0xFFF9FAFB);

        TextView title = new TextView(this);
        title.setText("EasyCodex");
        title.setTextSize(20);
        title.setTypeface(Typeface.DEFAULT_BOLD);
        title.setTextColor(0xFF111827);
        root.addView(title, matchWrap());

        LinearLayout connectRow = new LinearLayout(this);
        connectRow.setOrientation(LinearLayout.VERTICAL);
        connectRow.setPadding(0, dp(8), 0, dp(8));

        baseUrlInput = input("Agent URL", baseUrl);
        tokenInput = input("Token", token);
        connectRow.addView(baseUrlInput, matchWrap());
        connectRow.addView(tokenInput, matchWrap());

        LinearLayout actionRow = new LinearLayout(this);
        actionRow.setOrientation(LinearLayout.HORIZONTAL);
        Button connectButton = button("Test");
        Button saveButton = button("Save");
        Button refreshButton = button("Refresh");
        actionRow.addView(connectButton, weightWrap(1));
        actionRow.addView(saveButton, weightWrap(1));
        actionRow.addView(refreshButton, weightWrap(1));
        connectRow.addView(actionRow, matchWrap());

        LinearLayout sessionRow = new LinearLayout(this);
        sessionRow.setOrientation(LinearLayout.HORIZONTAL);
        Button newCodexButton = button("New Codex @ D:\\mgame");
        sessionRow.addView(newCodexButton, weightWrap(1));
        connectRow.addView(sessionRow, matchWrap());
        root.addView(connectRow, matchWrap());

        statusView = label("Idle");
        root.addView(statusView, matchWrap());

        lastSentView = smallLabel("Last sent: -");
        root.addView(lastSentView, matchWrap());

        panesView = new LinearLayout(this);
        panesView.setOrientation(LinearLayout.HORIZONTAL);
        panesView.setGravity(Gravity.CENTER_VERTICAL);
        HorizontalScrollView paneScroll = new HorizontalScrollView(this);
        paneScroll.addView(panesView);
        root.addView(paneScroll, fixedHeight(dp(64)));

        terminalScroll = new ScrollView(this);
        terminalView = new TextView(this);
        terminalView.setTextColor(0xFFE5E7EB);
        terminalView.setBackgroundColor(0xFF111827);
        terminalView.setTypeface(Typeface.MONOSPACE);
        terminalView.setTextSize(12);
        terminalView.setPadding(dp(8), dp(8), dp(8), dp(8));
        terminalView.setText("Connect to Agent, then select a pane.");
        terminalScroll.addView(terminalView, matchWrap());
        root.addView(terminalScroll, new LinearLayout.LayoutParams(-1, 0, 1));

        commandInput = multilineInput("Command", "");
        root.addView(commandInput, matchWrap());

        LinearLayout sendRow = new LinearLayout(this);
        sendRow.setOrientation(LinearLayout.HORIZONTAL);
        Button sendEnter = button("Send Enter");
        Button sendText = button("Send Text");
        Button clearText = button("Clear");
        sendRow.addView(sendEnter, weightWrap(1));
        sendRow.addView(sendText, weightWrap(1));
        sendRow.addView(clearText, weightWrap(1));
        root.addView(sendRow, matchWrap());

        LinearLayout keyRow = new LinearLayout(this);
        keyRow.setOrientation(LinearLayout.HORIZONTAL);
        Button enterOnly = button("Enter");
        Button ctrlC = button("Ctrl+C");
        Button tab = button("Tab");
        Button esc = button("Esc");
        keyRow.addView(enterOnly, weightWrap(1));
        keyRow.addView(ctrlC, weightWrap(1));
        keyRow.addView(tab, weightWrap(1));
        keyRow.addView(esc, weightWrap(1));
        root.addView(keyRow, matchWrap());

        setContentView(root);

        connectButton.setOnClickListener(v -> connect());
        saveButton.setOnClickListener(v -> saveConnection());
        refreshButton.setOnClickListener(v -> loadSessions());
        newCodexButton.setOnClickListener(v -> spawnCodexSession());
        sendEnter.setOnClickListener(v -> sendCommand(true));
        sendText.setOnClickListener(v -> sendCommand(false));
        clearText.setOnClickListener(v -> commandInput.setText(""));
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
            loadInstances();
        });
    }

    private void saveConnection() {
        baseUrl = trimTrailingSlash(baseUrlInput.getText().toString().trim());
        token = tokenInput.getText().toString().trim();
        saveSettings();
        setStatus("Saved");
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
            panesView.addView(item, new LinearLayout.LayoutParams(dp(150), dp(52)));
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
        setStatus("Starting Codex @ D:\\mgame...");
        try {
            JSONObject body = new JSONObject();
            JSONArray command = new JSONArray();
            command.put("cmd.exe");
            command.put("/k");
            command.put("cd /d D:\\mgame && codex --dangerously-bypass-approvals-and-sandbox");
            body.put("cwd", "D:\\mgame");
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
        input.setPadding(dp(8), 0, dp(8), 0);
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
        view.setTextSize(12);
        view.setSingleLine(true);
        view.setPadding(0, 0, 0, dp(6));
        return view;
    }

    private Button button(String text) {
        Button button = new Button(this);
        button.setText(text);
        button.setTextSize(12);
        return button;
    }

    private void stylePaneButton(Button button, boolean selected, boolean active) {
        button.setTextColor(selected ? 0xFFFFFFFF : 0xFF111827);
        if (selected) {
            button.setBackgroundColor(0xFF2563EB);
        } else if (active) {
            button.setBackgroundColor(0xFFD1FAE5);
        } else {
            button.setBackgroundColor(0xFFE5E7EB);
        }
    }

    private void setStatus(String text) {
        statusView.setText(text);
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
        return prefix + pane.id + " " + safeTitle(pane);
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
