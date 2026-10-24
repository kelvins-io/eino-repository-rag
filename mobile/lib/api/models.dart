class AuthUser {
  final String id;
  final String username;
  final String displayName;

  AuthUser({
    required this.id,
    required this.username,
    required this.displayName,
  });

  factory AuthUser.fromJson(Map<String, dynamic> json) => AuthUser(
        id: '${json['id'] ?? ''}',
        username: '${json['username'] ?? ''}',
        displayName: '${json['display_name'] ?? ''}',
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'username': username,
        'display_name': displayName,
      };
}

class AuthTenant {
  final int id;
  final String code;
  final String name;

  AuthTenant({required this.id, required this.code, required this.name});

  factory AuthTenant.fromJson(Map<String, dynamic> json) => AuthTenant(
        id: _asInt(json['id']),
        code: '${json['code'] ?? ''}',
        name: '${json['name'] ?? ''}',
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'code': code,
        'name': name,
      };
}

class AuthResult {
  final String token;
  final AuthUser user;
  final AuthTenant tenant;

  AuthResult({
    required this.token,
    required this.user,
    required this.tenant,
  });

  factory AuthResult.fromJson(Map<String, dynamic> json) => AuthResult(
        token: '${json['token'] ?? ''}',
        user: AuthUser.fromJson(
          Map<String, dynamic>.from(json['user'] as Map? ?? {}),
        ),
        tenant: AuthTenant.fromJson(
          Map<String, dynamic>.from(json['tenant'] as Map? ?? {}),
        ),
      );
}

class KnowledgeBase {
  final int id;
  final String name;
  final String description;

  KnowledgeBase({
    required this.id,
    required this.name,
    required this.description,
  });

  factory KnowledgeBase.fromJson(Map<String, dynamic> json) => KnowledgeBase(
        id: _asInt(json['id']),
        name: '${json['name'] ?? ''}',
        description: '${json['description'] ?? ''}',
      );
}

class DirectoryNode {
  final int id;
  final int knowledgeBaseId;
  final int? parentId;
  final String name;
  final String description;
  final List<DirectoryNode> children;

  DirectoryNode({
    required this.id,
    required this.knowledgeBaseId,
    this.parentId,
    required this.name,
    required this.description,
    this.children = const [],
  });

  factory DirectoryNode.fromJson(Map<String, dynamic> json) {
    final kids = (json['children'] as List?) ?? const [];
    return DirectoryNode(
      id: _asInt(json['id']),
      knowledgeBaseId: _asInt(json['knowledge_base_id']),
      parentId: json['parent_id'] == null ? null : _asInt(json['parent_id']),
      name: '${json['name'] ?? ''}',
      description: '${json['description'] ?? ''}',
      children: kids
          .map((e) => DirectoryNode.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(),
    );
  }
}

class DocumentItem {
  final int id;
  final String title;
  final String fileName;
  final String status;
  final String? errorMsg;

  DocumentItem({
    required this.id,
    required this.title,
    required this.fileName,
    required this.status,
    this.errorMsg,
  });

  factory DocumentItem.fromJson(Map<String, dynamic> json) => DocumentItem(
        id: _asInt(json['id']),
        title: '${json['title'] ?? ''}',
        fileName: '${json['file_name'] ?? ''}',
        status: '${json['status'] ?? ''}',
        errorMsg: json['error_msg']?.toString(),
      );
}

class ImportFileResult {
  final String fileName;
  final DocumentItem? document;
  final bool duplicated;
  final String message;

  ImportFileResult({
    required this.fileName,
    this.document,
    required this.duplicated,
    required this.message,
  });

  factory ImportFileResult.fromJson(Map<String, dynamic> json) {
    final doc = json['document'];
    return ImportFileResult(
      fileName: '${json['file_name'] ?? ''}',
      document: doc is Map
          ? DocumentItem.fromJson(Map<String, dynamic>.from(doc))
          : null,
      duplicated: json['duplicated'] == true,
      message: '${json['message'] ?? ''}',
    );
  }
}

class ImportResult {
  final List<ImportFileResult> items;
  final int total;
  final int imported;
  final int duplicated;
  final String message;

  ImportResult({
    required this.items,
    required this.total,
    required this.imported,
    required this.duplicated,
    required this.message,
  });

  factory ImportResult.fromJson(Map<String, dynamic> json) {
    final list = (json['items'] as List?) ?? const [];
    return ImportResult(
      items: list
          .map((e) => ImportFileResult.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(),
      total: _asInt(json['total']),
      imported: _asInt(json['imported']),
      duplicated: _asInt(json['duplicated']),
      message: '${json['message'] ?? ''}',
    );
  }
}

class SourceDocument {
  final String id;
  final String docId;
  final int chunkIndex;
  final int page;
  final String title;
  final String section;
  final String content;
  final double score;

  SourceDocument({
    required this.id,
    required this.docId,
    required this.chunkIndex,
    required this.page,
    required this.title,
    required this.section,
    required this.content,
    required this.score,
  });

  factory SourceDocument.fromJson(Map<String, dynamic> json) => SourceDocument(
        id: '${json['id'] ?? ''}',
        docId: '${json['doc_id'] ?? ''}',
        chunkIndex: _asInt(json['chunk_index']),
        page: _asInt(json['page']),
        title: '${json['title'] ?? ''}',
        section: '${json['section'] ?? ''}',
        content: '${json['content'] ?? ''}',
        score: (json['score'] is num) ? (json['score'] as num).toDouble() : 0,
      );
}

class StreamEvent {
  final String type;
  final String? content;
  final String? answer;
  final String? sessionId;
  final String? message;
  final List<SourceDocument> sources;
  final int? messageId;

  StreamEvent({
    required this.type,
    this.content,
    this.answer,
    this.sessionId,
    this.message,
    this.sources = const [],
    this.messageId,
  });

  factory StreamEvent.fromJson(Map<String, dynamic> json) {
    final src = (json['sources'] as List?) ?? const [];
    return StreamEvent(
      type: '${json['type'] ?? ''}',
      content: json['content']?.toString(),
      answer: json['answer']?.toString(),
      sessionId: json['session_id']?.toString(),
      message: json['message']?.toString(),
      messageId: json['message_id'] == null ? null : _asInt(json['message_id']),
      sources: src
          .map((e) => SourceDocument.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(),
    );
  }
}

class ChatMessage {
  final String role;
  String content;
  List<SourceDocument> sources;
  int? id;
  bool streaming;

  ChatMessage({
    required this.role,
    required this.content,
    this.sources = const [],
    this.id,
    this.streaming = false,
  });
}

int _asInt(dynamic v) {
  if (v is int) return v;
  if (v is num) return v.toInt();
  return int.tryParse('$v') ?? 0;
}
