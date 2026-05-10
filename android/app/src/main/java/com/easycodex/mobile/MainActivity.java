package com.easycodex.mobile;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.text.SpannableStringBuilder;
import android.text.Spanned;
import android.text.style.BackgroundColorSpan;
import android.text.style.ForegroundColorSpan;
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
import android.widget.Spinner;
import android.widget.TextView;
import android.widget.ArrayAdapter;
import android.widget.AdapterView;
import android.widget.Switch;

import org.json.JSONArray;
import org.json.JSONObject;

import com.google.zxing.integration.android.IntentIntegrator;
import com.google.zxing.integration.android.IntentResult;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Date;
import java.util.Locale;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends Activity {
    private static final int REQUEST_CAMERA_SCAN = 41;
    private static final int MAX_CONNECTION_HISTORY = 10;
    private static final int[] ANSI_COLORS = new int[]{
            0xFF0B1220, 0xFFDC2626, 0xFF16A34A, 0xFFD97706,
            0xFF2563EB, 0xFFC026D3, 0xFF0891B2, 0xFFE6EDF3,
            0xFF64748B, 0xFFEF4444, 0xFF22C55E, 0xFFF59E0B,
            0xFF60A5FA, 0xFFE879F9, 0xFF22D3EE, 0xFFFFFFFF
    };

    private final ExecutorService executor = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private final List<PaneInfo> panes = new ArrayList<>();
    private final List<ConnectionHistoryItem> connectionHistory = new ArrayList<>();

    private EditText baseUrlInput;
    private EditText tokenInput;
    private TextView statusView;
    private LinearLayout panesView;
    private TextView terminalView;
    private ScrollView terminalScroll;
    private EditText commandInput;
    private TextView lastSentView;
    private Button currentPaneButton;
    private View keyPanel;
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
    private boolean showTerminalColors = true;

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
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        IntentResult result = IntentIntegrator.parseActivityResult(requestCode, resultCode, data);
        if (result != null) {
            String contents = result.getContents();
            if (contents == null || contents.trim().isEmpty()) {
                setStatus("Scan cancelled");
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
                setStatus("Camera denied");
            }
        }
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
        topBar.addView(statusButton, rowWeightParams(1, dp(38), 0, dp(6)));
        topBar.addView(settingsButton, rowFixedParams(dp(44), dp(38), 0, 0));
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
        sessionStrip.addView(newCodexButton, rowFixedParams(dp(44), dp(42), 0, dp(6)));
        sessionStrip.addView(paneScroll, rowWeightParams(1, dp(42), 0, 0));
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

        LinearLayout specialKeys = new LinearLayout(this);
        specialKeys.setOrientation(LinearLayout.VERTICAL);
        specialKeys.setVisibility(View.GONE);
        LinearLayout keyRowOne = new LinearLayout(this);
        keyRowOne.setOrientation(LinearLayout.HORIZONTAL);
        keyRowOne.setGravity(Gravity.CENTER_VERTICAL);
        LinearLayout keyRowTwo = new LinearLayout(this);
        keyRowTwo.setOrientation(LinearLayout.HORIZONTAL);
        keyRowTwo.setGravity(Gravity.CENTER_VERTICAL);
        Button enterOnly = compactButton("Enter");
        Button ctrlC = compactButton("Ctrl+C");
        Button shiftTab = compactButton("S+Tab");
        Button shiftPageUp = compactButton("S+PgUp");
        Button shiftPageDown = compactButton("S+PgDn");
        Button space = compactButton("Space");
        Button up = compactButton("↑");
        Button down = compactButton("↓");
        Button esc = compactButton("Esc");
        Button clearText = compactButton("Clear");
        keyRowOne.addView(enterOnly, rowWeightParams(1, -1, 0, dp(5)));
        keyRowOne.addView(ctrlC, rowWeightParams(1, -1, 0, dp(5)));
        keyRowOne.addView(shiftTab, rowWeightParams(1, -1, 0, dp(5)));
        keyRowOne.addView(shiftPageUp, rowWeightParams(1, -1, 0, dp(5)));
        keyRowOne.addView(shiftPageDown, rowWeightParams(1, -1, 0, 0));
        keyRowTwo.addView(space, rowWeightParams(1, -1, 0, dp(5)));
        keyRowTwo.addView(up, rowWeightParams(1, -1, 0, dp(5)));
        keyRowTwo.addView(down, rowWeightParams(1, -1, 0, dp(5)));
        keyRowTwo.addView(esc, rowWeightParams(1, -1, 0, dp(5)));
        keyRowTwo.addView(clearText, rowWeightParams(1, -1, 0, 0));
        specialKeys.addView(keyRowOne, rowFixedParams(-1, dp(32), 0, 0));
        specialKeys.addView(keyRowTwo, rowFixedParams(-1, dp(32), 0, 0));
        keyPanel = specialKeys;
        root.addView(keyPanel, fixedHeight(dp(68)));

        LinearLayout inputRow = new LinearLayout(this);
        inputRow.setOrientation(LinearLayout.HORIZONTAL);
        inputRow.setGravity(Gravity.CENTER_VERTICAL);
        commandInput = multilineInput("Message", "");
        Button moreKeys = iconButton("⌘");
        Button sendEnter = button("Send");
        inputRow.addView(commandInput, rowWeightParams(1, dp(48), 0, dp(6)));
        inputRow.addView(moreKeys, rowFixedParams(dp(44), dp(48), 0, dp(6)));
        inputRow.addView(sendEnter, rowFixedParams(dp(76), dp(48), 0, 0));
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
        shiftTab.setOnClickListener(v -> sendRaw("\u001B[Z", false));
        shiftPageUp.setOnClickListener(v -> sendRaw("\u001B[5;2~", false));
        shiftPageDown.setOnClickListener(v -> sendRaw("\u001B[6;2~", false));
        space.setOnClickListener(v -> sendRaw(" ", false));
        up.setOnClickListener(v -> sendRaw("\u001B[A", false));
        down.setOnClickListener(v -> sendRaw("\u001B[B", false));
        esc.setOnClickListener(v -> sendRaw("\u001B", false));
    }

    private void connect() {
        saveConnection();
        setStatus("Connecting...");
        request("GET", "/api/health", null, false, result -> {
            if (!result.ok) {
                updateConnectionHistoryStatus(baseUrl, "fail");
                setStatus("Health failed: " + result.error);
                return;
            }
            updateConnectionHistoryStatus(baseUrl, "ok");
            loadRemoteConfig();
        });
    }

    private void saveConnection() {
        baseUrl = trimTrailingSlash(baseUrlInput.getText().toString().trim());
        token = tokenInput.getText().toString().trim();
        saveSettings();
        rememberConnection(baseUrl, token, "saved");
        setStatus("Saved");
    }

    private void showSettingsDialog() {
        LinearLayout panel = new LinearLayout(this);
        panel.setOrientation(LinearLayout.VERTICAL);
        panel.setPadding(dp(18), dp(16), dp(18), dp(14));
        panel.setBackground(rounded(0xFFFFFFFF, dp(12), 0));

        TextView title = new TextView(this);
        title.setText("服务器连接");
        title.setTextSize(18);
        title.setTypeface(Typeface.DEFAULT_BOLD);
        title.setTextColor(0xFF111827);
        panel.addView(title, matchWrap());

        TextView hint = smallLabel("扫码 PC 配对二维码，或手动修改 Agent 地址和 Token。");
        hint.setSingleLine(false);
        hint.setPadding(0, dp(4), 0, dp(12));
        panel.addView(hint, matchWrap());

        EditText urlField = input("Agent URL", baseUrl);
        EditText tokenField = input("Token", token);
        Switch colorSwitch = new Switch(this);
        colorSwitch.setText("显示终端颜色");
        colorSwitch.setTextSize(14);
        colorSwitch.setTextColor(0xFF344054);
        colorSwitch.setChecked(showTerminalColors);
        Spinner historySpinner = new Spinner(this);
        List<String> historyLabels = connectionHistoryLabels();
        ArrayAdapter<String> historyAdapter = new ArrayAdapter<>(this, android.R.layout.simple_spinner_item, historyLabels);
        historyAdapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item);
        historySpinner.setAdapter(historyAdapter);
        panel.addView(fieldLabel("Recent Connections"), matchWrap());
        panel.addView(historySpinner, fixedHeight(dp(44)));
        panel.addView(fieldSpacer(), fixedHeight(dp(10)));
        panel.addView(fieldLabel("Agent 地址"), matchWrap());
        panel.addView(urlField, fixedHeight(dp(44)));
        panel.addView(fieldSpacer(), fixedHeight(dp(10)));
        panel.addView(fieldLabel("配对 Token"), matchWrap());
        panel.addView(tokenField, fixedHeight(dp(44)));
        panel.addView(fieldSpacer(), fixedHeight(dp(10)));
        panel.addView(colorSwitch, fixedHeight(dp(40)));

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
            applyConnectionFields(urlField, tokenField);
            applyDisplaySettings(colorSwitch);
            saveConnection();
            dialog.dismiss();
        });
        connectButton.setOnClickListener(v -> {
            applyConnectionFields(urlField, tokenField);
            applyDisplaySettings(colorSwitch);
            connect();
            dialog.dismiss();
        });
        dialog.show();
    }

    private void applyConnectionFields(EditText urlField, EditText tokenField) {
        baseUrlInput.setText(urlField.getText().toString());
        tokenInput.setText(tokenField.getText().toString());
    }

    private void applyDisplaySettings(Switch colorSwitch) {
        boolean nextShowTerminalColors = colorSwitch.isChecked();
        if (showTerminalColors == nextShowTerminalColors) {
            return;
        }
        showTerminalColors = nextShowTerminalColors;
        saveDisplaySettings();
        snapshotHash = "";
        if (!lastSnapshotText.isEmpty()) {
            renderTerminalText(lastSnapshotText);
        }
        if (!paneId.isEmpty()) {
            pollSnapshot(pollToken);
        }
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
            panesView.addView(item, rowFixedParams(dp(132), dp(38), 0, dp(6)));
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
        if (showTerminalColors) {
            path += "&escapes=1";
        }
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
                renderTerminalText(lastSnapshotText);
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
        handlePairingUri(intent.getData());
    }

    private boolean handlePairingUri(Uri uri) {
        if (!"easycodex".equals(uri.getScheme()) || !"pair".equals(uri.getHost())) {
            return false;
        }
        String pairUrl = uri.getQueryParameter("url");
        if (pairUrl == null || pairUrl.isEmpty()) {
            pairUrl = uri.getQueryParameter("u");
        }
        if (pairUrl != null && !pairUrl.isEmpty()) {
            final String finalPairUrl = pairUrl;
            saveScannedBaseUrl(baseUrlFromPairEndpoint(finalPairUrl));
            setStatus("Pairing from PC...");
            requestAbsolute(finalPairUrl, result -> {
                if (!result.ok) {
                    updateConnectionHistoryStatus(baseUrlFromPairEndpoint(finalPairUrl), "fail");
                    setStatus("Pairing failed: " + result.error);
                    return;
                }
                applyPairingPayload(result.data);
                connect();
            });
            return true;
        }
        String data = uri.getQueryParameter("data");
        if (data == null || data.isEmpty()) {
            setStatus("Pairing failed: missing data");
            return true;
        }
        try {
            String json = new String(Base64.decode(data, Base64.DEFAULT), StandardCharsets.UTF_8);
            applyPairingPayload(new JSONObject(json));
            connect();
        } catch (Exception ex) {
            setStatus("Pairing failed: " + ex.getMessage());
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
                setStatus("Pairing from QR...");
                requestAbsolute(contents, result -> {
                    if (!result.ok) {
                        updateConnectionHistoryStatus(baseUrlFromPairEndpoint(contents), "fail");
                        setStatus("Pairing failed: " + result.error);
                        return;
                    }
                    applyPairingPayload(result.data);
                    connect();
                });
                return;
            }
            if ("http".equals(scheme) || "https".equals(scheme)) {
                saveScannedBaseUrl(contents);
                setStatus("Paired " + baseUrl);
                connect();
                return;
            }
            setStatus("Pairing failed: unsupported QR");
        } catch (Exception ex) {
            setStatus("Pairing failed: " + ex.getMessage());
        }
    }

    private void saveScannedBaseUrl(String scannedBaseUrl) {
        String nextBaseUrl = trimTrailingSlash(scannedBaseUrl == null ? "" : scannedBaseUrl.trim());
        if (nextBaseUrl.isEmpty()) {
            return;
        }
        baseUrl = nextBaseUrl;
        baseUrlInput.setText(baseUrl);
        tokenInput.setText(token);
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
        rememberConnection(baseUrl, token, "ok");
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
        showTerminalColors = prefs.getBoolean("showTerminalColors", showTerminalColors);
        loadConnectionHistory(prefs);
    }

    private void saveSettings() {
        getSharedPreferences("easycodex", MODE_PRIVATE)
                .edit()
                .putString("baseUrl", baseUrl)
                .putString("token", token)
                .apply();
    }

    private void saveDisplaySettings() {
        getSharedPreferences("easycodex", MODE_PRIVATE)
                .edit()
                .putBoolean("showTerminalColors", showTerminalColors)
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
        String normalizedUrl = trimTrailingSlash(url.trim());
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
        labels.add(connectionHistory.isEmpty() ? "No recent connections" : "Select recent connection");
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

    private void renderTerminalText(String text) {
        if (showTerminalColors) {
            terminalView.setText(ansiToSpannable(text));
        } else {
            terminalView.setText(stripAnsi(text));
        }
    }

    private CharSequence ansiToSpannable(String text) {
        SpannableStringBuilder builder = new SpannableStringBuilder();
        int fg = -1;
        int bg = -1;
        int index = 0;
        int segmentStart = 0;
        while (index < text.length()) {
            if (text.charAt(index) == 27) {
                int sequenceEnd = findAnsiSequenceEnd(text, index);
                if (sequenceEnd > index) {
                    appendAnsiRun(builder, text, segmentStart, index, fg, bg);
                    if (isSgrSequence(text, index, sequenceEnd)) {
                        int[] next = applySgr(text.substring(index + 2, sequenceEnd - 1), fg, bg);
                        fg = next[0];
                        bg = next[1];
                    }
                    index = sequenceEnd;
                    segmentStart = index;
                    continue;
                }
            }
            index++;
        }
        appendAnsiRun(builder, text, segmentStart, text.length(), fg, bg);
        return builder;
    }

    private String stripAnsi(String text) {
        StringBuilder builder = new StringBuilder();
        int index = 0;
        while (index < text.length()) {
            if (text.charAt(index) == 27) {
                int sequenceEnd = findAnsiSequenceEnd(text, index);
                if (sequenceEnd > index) {
                    index = sequenceEnd;
                    continue;
                }
            }
            builder.append(text.charAt(index));
            index++;
        }
        return builder.toString();
    }

    private int findAnsiSequenceEnd(String text, int escapeIndex) {
        if (escapeIndex + 1 >= text.length()) {
            return -1;
        }
        char next = text.charAt(escapeIndex + 1);
        if (next == '[') {
            int commandEnd = findAnsiCommandEnd(text, escapeIndex + 2);
            return commandEnd >= 0 ? commandEnd + 1 : text.length();
        }
        if (next == ']' || next == 'P' || next == '_' || next == '^' || next == 'X') {
            return findStringControlEnd(text, escapeIndex + 2);
        }
        if (next == '(' || next == ')' || next == '*' || next == '+' || next == '-' || next == '.' || next == '/' || next == '#') {
            return Math.min(text.length(), escapeIndex + 3);
        }
        return Math.min(text.length(), escapeIndex + 2);
    }

    private boolean isSgrSequence(String text, int escapeIndex, int sequenceEnd) {
        return escapeIndex + 2 < sequenceEnd
                && text.charAt(escapeIndex + 1) == '['
                && text.charAt(sequenceEnd - 1) == 'm';
    }

    private int findStringControlEnd(String text, int start) {
        for (int i = start; i < text.length(); i++) {
            char ch = text.charAt(i);
            if (ch == 7) {
                return i + 1;
            }
            if (ch == 27 && i + 1 < text.length() && text.charAt(i + 1) == '\\') {
                return i + 2;
            }
        }
        return text.length();
    }

    private int findAnsiCommandEnd(String text, int start) {
        for (int i = start; i < text.length(); i++) {
            char ch = text.charAt(i);
            if (ch >= '@' && ch <= '~') {
                return i;
            }
        }
        return -1;
    }

    private void appendAnsiRun(SpannableStringBuilder builder, String text, int start, int end, int fg, int bg) {
        if (end <= start) {
            return;
        }
        int spanStart = builder.length();
        builder.append(text, start, end);
        int spanEnd = builder.length();
        if (fg != -1) {
            builder.setSpan(new ForegroundColorSpan(fg), spanStart, spanEnd, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
        }
        if (bg != -1) {
            builder.setSpan(new BackgroundColorSpan(bg), spanStart, spanEnd, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
        }
    }

    private int[] applySgr(String params, int fg, int bg) {
        if (params.isEmpty()) {
            return new int[]{-1, -1};
        }
        String[] parts = params.split(";");
        for (int i = 0; i < parts.length; i++) {
            int code = parseAnsiCode(parts[i], 0);
            if (code == 0) {
                fg = -1;
                bg = -1;
            } else if (code == 39) {
                fg = -1;
            } else if (code == 49) {
                bg = -1;
            } else if (code >= 30 && code <= 37) {
                fg = ansiColor(code - 30);
            } else if (code >= 40 && code <= 47) {
                bg = ansiColor(code - 40);
            } else if (code >= 90 && code <= 97) {
                fg = ansiColor(code - 90 + 8);
            } else if (code >= 100 && code <= 107) {
                bg = ansiColor(code - 100 + 8);
            } else if ((code == 38 || code == 48) && i + 2 < parts.length) {
                boolean foreground = code == 38;
                int mode = parseAnsiCode(parts[++i], -1);
                if (mode == 5 && i + 1 < parts.length) {
                    int color = xtermColor(parseAnsiCode(parts[++i], -1));
                    if (foreground) {
                        fg = color;
                    } else {
                        bg = color;
                    }
                } else if (mode == 2 && i + 3 < parts.length) {
                    int r = clampColor(parseAnsiCode(parts[++i], 0));
                    int g = clampColor(parseAnsiCode(parts[++i], 0));
                    int b = clampColor(parseAnsiCode(parts[++i], 0));
                    int color = 0xFF000000 | (r << 16) | (g << 8) | b;
                    if (foreground) {
                        fg = color;
                    } else {
                        bg = color;
                    }
                }
            }
        }
        return new int[]{fg, bg};
    }

    private int parseAnsiCode(String text, int fallback) {
        try {
            return Integer.parseInt(text.trim());
        } catch (Exception ignored) {
            return fallback;
        }
    }

    private int ansiColor(int index) {
        if (index < 0 || index >= ANSI_COLORS.length) {
            return -1;
        }
        return ANSI_COLORS[index];
    }

    private int xtermColor(int index) {
        if (index >= 0 && index < 16) {
            return ansiColor(index);
        }
        if (index >= 16 && index <= 231) {
            int value = index - 16;
            int r = value / 36;
            int g = (value / 6) % 6;
            int b = value % 6;
            return 0xFF000000 | (xtermChannel(r) << 16) | (xtermChannel(g) << 8) | xtermChannel(b);
        }
        if (index >= 232 && index <= 255) {
            int level = 8 + (index - 232) * 10;
            return 0xFF000000 | (level << 16) | (level << 8) | level;
        }
        return -1;
    }

    private int xtermChannel(int value) {
        return value == 0 ? 0 : 55 + value * 40;
    }

    private int clampColor(int value) {
        return Math.max(0, Math.min(255, value));
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

    private TextView fieldLabel(String text) {
        TextView view = new TextView(this);
        view.setText(text);
        view.setTextColor(0xFF374151);
        view.setTextSize(12);
        view.setTypeface(Typeface.DEFAULT_BOLD);
        view.setPadding(0, 0, 0, dp(4));
        return view;
    }

    private View fieldSpacer() {
        View view = new View(this);
        view.setBackgroundColor(0x00000000);
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
        button.setTextSize(10);
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
        statusView.setText(statusLabel(text));
        styleStatus(text);
    }

    private String statusLabel(String text) {
        String value = text == null ? "" : text.toLowerCase();
        if (value.contains("connected") || value.contains("configured") || value.startsWith("panes:") || value.contains(" selected")) {
            return "Connected";
        }
        if (value.contains("connecting")) {
            return "Connecting";
        }
        if (value.contains("failed") || value.contains("error") || value.contains("unavailable")) {
            return "Error";
        }
        if (value.contains("sending") || value.contains("starting") || value.contains("loading")) {
            return "Working";
        }
        if (value.contains("sent") || value.contains("started") || value.contains("saved")) {
            return "Ready";
        }
        return "Offline";
    }

    private void styleStatus(String text) {
        if (statusView == null) {
            return;
        }
        String value = text == null ? "" : text.toLowerCase();
        int fill = 0xFFE5E7EB;
        int stroke = 0xFFD1D5DB;
        int textColor = 0xFF374151;
        if (value.contains("connected") || value.contains("configured") || value.contains("sent") || value.contains("started") || value.contains("saved") || value.startsWith("panes:") || value.contains(" selected")) {
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

    private LinearLayout.LayoutParams rowWeightParams(float weight, int height, int left, int right) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(0, height, weight);
        params.setMargins(left, 0, right, 0);
        return params;
    }

    private LinearLayout.LayoutParams rowFixedParams(int width, int height, int left, int right) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(width, height);
        params.setMargins(left, 0, right, 0);
        return params;
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
