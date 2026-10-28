import 'dart:async';
import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:http/http.dart' as http;

import 'api_exception.dart';
import 'models.dart';

typedef UnauthorizedHandler = void Function();

class ApiClient {
  ApiClient({
    required String baseUrl,
    String? token,
    this.onUnauthorized,
  }) {
    setBaseUrl(baseUrl);
    setToken(token);
  }

  UnauthorizedHandler? onUnauthorized;

  late Dio _dio;
  String _baseUrl = '';
  String? _token;

  String get baseUrl => _baseUrl;
  String? get token => _token;

  void setBaseUrl(String url) {
    _baseUrl = url.trim().replaceAll(RegExp(r'/+$'), '');
    _dio = Dio(
      BaseOptions(
        baseUrl: _baseUrl,
        connectTimeout: const Duration(seconds: 20),
        receiveTimeout: const Duration(seconds: 120),
        headers: {'Accept': 'application/json'},
      ),
    );
    _dio.interceptors.add(
      InterceptorsWrapper(
        onRequest: (options, handler) {
          if (_token != null && _token!.isNotEmpty) {
            options.headers['Authorization'] = 'Bearer $_token';
          }
          handler.next(options);
        },
        onError: (err, handler) {
          final status = err.response?.statusCode;
          final msg = _extractMessage(err.response?.data) ??
              err.message ??
              '网络错误';
          if (_shouldForceLogout(status, msg)) {
            onUnauthorized?.call();
          }
          handler.reject(
            DioException(
              requestOptions: err.requestOptions,
              response: err.response,
              type: err.type,
              error: ApiException(
                msg,
                statusCode: status,
                forceLogout: _shouldForceLogout(status, msg),
              ),
            ),
          );
        },
      ),
    );
  }

  void setToken(String? token) {
    _token = token;
  }

  bool _shouldForceLogout(int? status, String msg) {
    return status == 401 || (status == 403 && msg.contains('禁止登录'));
  }

  String? _extractMessage(dynamic data) {
    if (data is Map && data['message'] != null) {
      return '${data['message']}';
    }
    if (data is String && data.isNotEmpty) return data;
    return null;
  }

  dynamic _unwrap(Response res) {
    final body = res.data;
    if (body is Map && body['code'] is num) {
      final code = (body['code'] as num).toInt();
      if (code != 0) {
        final msg = '${body['message'] ?? '请求失败'}';
        if (_shouldForceLogout(res.statusCode, msg)) {
          onUnauthorized?.call();
        }
        throw ApiException(msg, statusCode: res.statusCode);
      }
      return body['data'];
    }
    return body;
  }

  Future<T> _get<T>(
    String path, {
    Map<String, dynamic>? query,
    T Function(dynamic)? parse,
  }) async {
    try {
      final res = await _dio.get(path, queryParameters: query);
      final data = _unwrap(res);
      if (parse != null) return parse(data);
      return data as T;
    } on DioException catch (e) {
      throw _asApiException(e);
    }
  }

  Future<T> _post<T>(
    String path, {
    dynamic data,
    T Function(dynamic)? parse,
  }) async {
    try {
      final res = await _dio.post(path, data: data);
      final out = _unwrap(res);
      if (parse != null) return parse(out);
      return out as T;
    } on DioException catch (e) {
      throw _asApiException(e);
    }
  }

  ApiException _asApiException(DioException e) {
    if (e.error is ApiException) return e.error as ApiException;
    final status = e.response?.statusCode;
    final msg = _extractMessage(e.response?.data) ?? e.message ?? '网络错误';
    return ApiException(
      msg,
      statusCode: status,
      forceLogout: _shouldForceLogout(status, msg),
    );
  }

  Future<Map<String, dynamic>> health() async {
    try {
      final res = await _dio.get('/health');
      if (res.data is Map) {
        return Map<String, dynamic>.from(res.data as Map);
      }
      return {'status': 'ok'};
    } on DioException catch (e) {
      throw _asApiException(e);
    }
  }

  Future<AuthResult> login({
    required String tenantId,
    required String username,
    required String password,
  }) {
    return _post(
      '/api/v1/auth/login',
      data: {
        'tenant_id': tenantId,
        'username': username,
        'password': password,
      },
      parse: (d) => AuthResult.fromJson(Map<String, dynamic>.from(d as Map)),
    );
  }

  Future<List<KnowledgeBase>> listKnowledgeBases({
    int page = 1,
    int pageSize = 100,
  }) {
    return _get(
      '/api/v1/knowledge-bases',
      query: {'page': page, 'page_size': pageSize},
      parse: (d) {
        final map = Map<String, dynamic>.from(d as Map);
        final list = (map['list'] as List?) ?? const [];
        return list
            .map((e) => KnowledgeBase.fromJson(Map<String, dynamic>.from(e as Map)))
            .toList();
      },
    );
  }

  Future<List<DirectoryNode>> listDirectories(int kbId) {
    return _get(
      '/api/v1/knowledge-bases/$kbId/directories',
      parse: (d) {
        final list = (d as List?) ?? const [];
        return list
            .map((e) => DirectoryNode.fromJson(Map<String, dynamic>.from(e as Map)))
            .toList();
      },
    );
  }

  Future<ImportResult> importDocuments({
    required int knowledgeBaseId,
    int? directoryId,
    String? title,
    String? source,
    required List<MultipartFile> files,
  }) async {
    final form = FormData();
    form.fields.add(MapEntry('knowledge_base_id', '$knowledgeBaseId'));
    if (directoryId != null) {
      form.fields.add(MapEntry('directory_id', '$directoryId'));
    }
    final titleValue = title?.trim() ?? '';
    final sourceValue = source?.trim() ?? '';
    for (final f in files) {
      // 与 files 一一对应；手机端目前共用同一标题/来源
      form.fields.add(MapEntry('title', titleValue));
      form.fields.add(MapEntry('source', sourceValue));
      form.files.add(MapEntry('files', f));
    }
    try {
      final res = await _dio.post(
        '/api/v1/documents/import',
        data: form,
        options: Options(
          sendTimeout: const Duration(minutes: 5),
          receiveTimeout: const Duration(minutes: 5),
        ),
      );
      final data = _unwrap(res);
      return ImportResult.fromJson(Map<String, dynamic>.from(data as Map));
    } on DioException catch (e) {
      throw _asApiException(e);
    }
  }

  Future<List<ChatMessage>> chatHistory(String sessionId) {
    return _get(
      '/api/v1/chat/history',
      query: {'session_id': sessionId},
      parse: (d) {
        final list = (d as List?) ?? const [];
        return list.map((e) {
          final m = Map<String, dynamic>.from(e as Map);
          return ChatMessage(
            role: '${m['role'] ?? ''}',
            content: '${m['content'] ?? ''}',
            id: m['id'] == null ? null : int.tryParse('${m['id']}'),
          );
        }).toList();
      },
    );
  }

  /// 消费 POST /api/v1/chat/query 的 SSE 流。
  /// [isCancelled] 返回 true 时提前结束并关闭连接。
  Stream<StreamEvent> chatQueryStream({
    required String query,
    required int knowledgeBaseId,
    String? sessionId,
    int? directoryId,
    bool Function()? isCancelled,
  }) async* {
    final uri = Uri.parse('$_baseUrl/api/v1/chat/query');
    final headers = <String, String>{
      'Content-Type': 'application/json',
      'Accept': 'text/event-stream',
    };
    if (_token != null && _token!.isNotEmpty) {
      headers['Authorization'] = 'Bearer $_token';
    }
    final body = <String, dynamic>{
      'query': query,
      'knowledge_base_id': knowledgeBaseId,
      if (sessionId != null && sessionId.isNotEmpty) 'session_id': sessionId,
      if (directoryId != null) 'directory_id': directoryId,
    };

    final client = http.Client();
    try {
      final request = http.Request('POST', uri)
        ..headers.addAll(headers)
        ..body = jsonEncode(body);
      final response = await client.send(request).timeout(
            const Duration(seconds: 30),
          );

      if (response.statusCode < 200 || response.statusCode >= 300) {
        final raw = await response.stream.bytesToString();
        String msg = '请求失败 (${response.statusCode})';
        try {
          final parsed = jsonDecode(raw);
          if (parsed is Map && parsed['message'] != null) {
            msg = '${parsed['message']}';
          }
        } catch (_) {}
        if (_shouldForceLogout(response.statusCode, msg)) {
          onUnauthorized?.call();
        }
        throw ApiException(
          msg,
          statusCode: response.statusCode,
          forceLogout: _shouldForceLogout(response.statusCode, msg),
        );
      }

      final buffer = StringBuffer();
      await for (final chunk in response.stream.transform(utf8.decoder)) {
        if (isCancelled?.call() == true) break;
        buffer.write(chunk);
        var text = buffer.toString();
        var sep = text.indexOf('\n\n');
        while (sep >= 0) {
          final raw = text.substring(0, sep);
          text = text.substring(sep + 2);
          for (final line in raw.split('\n')) {
            final trimmed = line.trim();
            if (!trimmed.startsWith('data:')) continue;
            final payload = trimmed.substring(5).trim();
            if (payload.isEmpty || payload == '[DONE]') continue;
            try {
              final evt = StreamEvent.fromJson(
                Map<String, dynamic>.from(jsonDecode(payload) as Map),
              );
              yield evt;
              if (evt.type == 'error') {
                throw ApiException(evt.message ?? '流式回答失败');
              }
            } on ApiException {
              rethrow;
            } catch (_) {
              // ignore malformed frames
            }
          }
          sep = text.indexOf('\n\n');
        }
        buffer
          ..clear()
          ..write(text);
      }
    } finally {
      client.close();
    }
  }
}
