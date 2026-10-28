import 'dart:async';
import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/models.dart';

class AppState extends ChangeNotifier {
  static const _kBaseUrl = 'server_base_url';
  static const _kToken = 'token';
  static const _kUser = 'user';
  static const _kTenant = 'tenant';
  static const _kTenantPrefill = 'tenant_id_prefill';
  static const _kKbId = 'default_kb_id';
  static const _kDirId = 'default_directory_id';

  late final ApiClient api;
  SharedPreferences? _prefs;

  bool ready = false;
  String baseUrl = 'http://127.0.0.1:8080';
  String? token;
  AuthUser? user;
  AuthTenant? tenant;
  String tenantPrefill = 'guest';
  int? defaultKbId;
  int? defaultDirectoryId;

  List<KnowledgeBase> knowledgeBases = [];
  List<DirectoryNode> directories = [];
  bool loadingKbs = false;

  bool get isLoggedIn => token != null && token!.isNotEmpty;

  Future<void> init() async {
    _prefs = await SharedPreferences.getInstance();
    baseUrl = _prefs!.getString(_kBaseUrl) ?? _defaultBaseUrl();
    token = _prefs!.getString(_kToken);
    tenantPrefill = _prefs!.getString(_kTenantPrefill) ?? 'guest';
    defaultKbId = _prefs!.getInt(_kKbId);
    defaultDirectoryId = _prefs!.getInt(_kDirId);

    final userRaw = _prefs!.getString(_kUser);
    if (userRaw != null) {
      try {
        user = AuthUser.fromJson(jsonDecode(userRaw) as Map<String, dynamic>);
      } catch (_) {}
    }
    final tenantRaw = _prefs!.getString(_kTenant);
    if (tenantRaw != null) {
      try {
        tenant =
            AuthTenant.fromJson(jsonDecode(tenantRaw) as Map<String, dynamic>);
      } catch (_) {}
    }

    api = ApiClient(
      baseUrl: baseUrl,
      token: token,
      onUnauthorized: () {
        logout(notify: true);
      },
    );
    ready = true;
    notifyListeners();
    if (isLoggedIn) {
      unawaited(refreshKnowledgeBases());
    }
  }

  String _defaultBaseUrl() {
    // Android 模拟器访问宿主机用 10.0.2.2；真机请在设置里改成局域网 IP。
    if (defaultTargetPlatform == TargetPlatform.android) {
      return 'http://10.0.2.2:8080';
    }
    return 'http://127.0.0.1:8080';
  }

  Future<void> setBaseUrl(String url) async {
    baseUrl = url.trim().replaceAll(RegExp(r'/+$'), '');
    await _prefs?.setString(_kBaseUrl, baseUrl);
    api.setBaseUrl(baseUrl);
    notifyListeners();
  }

  Future<Map<String, dynamic>> probeHealth() => api.health();

  Future<void> login({
    required String tenantId,
    required String username,
    required String password,
  }) async {
    final result = await api.login(
      tenantId: tenantId,
      username: username,
      password: password,
    );
    token = result.token;
    user = result.user;
    tenant = result.tenant;
    tenantPrefill = tenantId;
    api.setToken(token);
    await _prefs?.setString(_kToken, token!);
    await _prefs?.setString(_kUser, jsonEncode(user!.toJson()));
    await _prefs?.setString(_kTenant, jsonEncode(tenant!.toJson()));
    await _prefs?.setString(_kTenantPrefill, tenantPrefill);
    notifyListeners();
    await refreshKnowledgeBases();
  }

  Future<void> logout({bool notify = true}) async {
    token = null;
    user = null;
    tenant = null;
    knowledgeBases = [];
    directories = [];
    api.setToken(null);
    await _prefs?.remove(_kToken);
    await _prefs?.remove(_kUser);
    await _prefs?.remove(_kTenant);
    if (notify) notifyListeners();
  }

  Future<void> setDefaultKb(int? id) async {
    defaultKbId = id;
    if (id == null) {
      await _prefs?.remove(_kKbId);
    } else {
      await _prefs?.setInt(_kKbId, id);
    }
    // 切换知识库时清空目录默认值
    await setDefaultDirectory(null, notify: false);
    notifyListeners();
    if (id != null) {
      await refreshDirectories(id);
    } else {
      directories = [];
      notifyListeners();
    }
  }

  Future<void> setDefaultDirectory(int? id, {bool notify = true}) async {
    defaultDirectoryId = id;
    if (id == null) {
      await _prefs?.remove(_kDirId);
    } else {
      await _prefs?.setInt(_kDirId, id);
    }
    if (notify) notifyListeners();
  }

  Future<void> refreshKnowledgeBases() async {
    if (!isLoggedIn) return;
    loadingKbs = true;
    notifyListeners();
    try {
      knowledgeBases = await api.listKnowledgeBases();
      if (defaultKbId != null &&
          !knowledgeBases.any((k) => k.id == defaultKbId)) {
        await setDefaultKb(null);
      } else if (defaultKbId != null) {
        await refreshDirectories(defaultKbId!);
      }
    } on ApiException {
      rethrow;
    } finally {
      loadingKbs = false;
      notifyListeners();
    }
  }

  Future<void> refreshDirectories(int kbId) async {
    directories = await api.listDirectories(kbId);
    if (defaultDirectoryId != null) {
      final found = _findDir(directories, defaultDirectoryId!);
      if (found == null) {
        await setDefaultDirectory(null, notify: false);
      }
    }
    notifyListeners();
  }

  DirectoryNode? _findDir(List<DirectoryNode> nodes, int id) {
    for (final n in nodes) {
      if (n.id == id) return n;
      final c = _findDir(n.children, id);
      if (c != null) return c;
    }
    return null;
  }

  String? kbName(int? id) {
    if (id == null) return null;
    for (final k in knowledgeBases) {
      if (k.id == id) return k.name;
    }
    return null;
  }

  String? directoryName(int? id) {
    if (id == null) return null;
    return _findDir(directories, id)?.name;
  }

  Future<ImportResult> uploadFiles({
    required int knowledgeBaseId,
    int? directoryId,
    String? title,
    String? source,
    required List<MultipartFile> files,
  }) {
    return api.importDocuments(
      knowledgeBaseId: knowledgeBaseId,
      directoryId: directoryId,
      title: title,
      source: source,
      files: files,
    );
  }

  Stream<StreamEvent> chatQuery({
    required String query,
    required int knowledgeBaseId,
    String? sessionId,
    int? directoryId,
    bool Function()? isCancelled,
  }) {
    return api.chatQueryStream(
      query: query,
      knowledgeBaseId: knowledgeBaseId,
      sessionId: sessionId,
      directoryId: directoryId,
      isCancelled: isCancelled,
    );
  }
}
