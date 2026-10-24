import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:uuid/uuid.dart';

import '../api/api_exception.dart';
import '../api/models.dart';
import '../services/app_state.dart';
import '../theme/app_theme.dart';
import '../widgets/kb_picker.dart';
import '../widgets/message_bubble.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key});

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _input = TextEditingController();
  final _scroll = ScrollController();
  final _uuid = const Uuid();

  final List<ChatMessage> _messages = [];
  String _sessionId = '';
  int? _kbId;
  int? _dirId;
  bool _sending = false;
  bool _cancelled = false;

  @override
  void initState() {
    super.initState();
    _sessionId = _uuid.v4();
    final s = context.read<AppState>();
    _kbId = s.defaultKbId;
    _dirId = s.defaultDirectoryId;
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      final state = context.read<AppState>();
      if (state.knowledgeBases.isEmpty) {
        try {
          await state.refreshKnowledgeBases();
        } catch (_) {}
      }
      if (!mounted) return;
      setState(() {
        _kbId ??= state.defaultKbId;
        _dirId ??= state.defaultDirectoryId;
      });
      if (_kbId != null && state.directories.isEmpty) {
        try {
          await state.refreshDirectories(_kbId!);
        } catch (_) {}
      }
    });
  }

  @override
  void dispose() {
    _cancelled = true;
    _input.dispose();
    _scroll.dispose();
    super.dispose();
  }

  void _newSession() {
    setState(() {
      _cancelled = true;
      _sending = false;
      _messages.clear();
      _sessionId = _uuid.v4();
      _cancelled = false;
    });
  }

  Future<void> _pickScope() async {
    final state = context.read<AppState>();
    var kb = _kbId;
    var dir = _dirId;
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (ctx) {
        return StatefulBuilder(
          builder: (ctx, setModal) {
            return Padding(
              padding: EdgeInsets.fromLTRB(
                16,
                8,
                16,
                16 + MediaQuery.of(ctx).viewInsets.bottom,
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text('检索范围', style: Theme.of(ctx).textTheme.titleMedium),
                  const SizedBox(height: 12),
                  KnowledgeBaseSelector(
                    items: state.knowledgeBases,
                    value: kb,
                    onChanged: (id) async {
                      setModal(() {
                        kb = id;
                        dir = null;
                      });
                      if (id != null) {
                        try {
                          await state.refreshDirectories(id);
                          setModal(() {});
                        } catch (_) {}
                      }
                    },
                  ),
                  const SizedBox(height: 12),
                  DirectorySelector(
                    tree: state.directories,
                    value: dir,
                    onChanged: (id) => setModal(() => dir = id),
                    enabled: kb != null,
                  ),
                  const SizedBox(height: 16),
                  FilledButton(
                    onPressed: () {
                      setState(() {
                        _kbId = kb;
                        _dirId = dir;
                      });
                      Navigator.pop(ctx);
                    },
                    child: const Text('应用范围'),
                  ),
                  const SizedBox(height: 8),
                ],
              ),
            );
          },
        );
      },
    );
  }

  Future<void> _send() async {
    final q = _input.text.trim();
    if (q.isEmpty || _sending) return;
    if (_kbId == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('请先选择知识库')),
      );
      await _pickScope();
      return;
    }

    setState(() {
      _sending = true;
      _cancelled = false;
      _messages.add(ChatMessage(role: 'user', content: q));
      _messages.add(ChatMessage(role: 'assistant', content: '', streaming: true));
      _input.clear();
    });
    _scrollToBottom();

    final assistantIdx = _messages.length - 1;
    final state = context.read<AppState>();

    try {
      await for (final evt in state.chatQuery(
        query: q,
        knowledgeBaseId: _kbId!,
        sessionId: _sessionId,
        directoryId: _dirId,
        isCancelled: () => _cancelled,
      )) {
        if (!mounted || _cancelled) break;
        switch (evt.type) {
          case 'meta':
            if (evt.sessionId != null && evt.sessionId!.isNotEmpty) {
              _sessionId = evt.sessionId!;
            }
            if (evt.sources.isNotEmpty) {
              setState(() => _messages[assistantIdx].sources = evt.sources);
            }
            break;
          case 'delta':
            setState(() {
              _messages[assistantIdx].content += evt.content ?? '';
            });
            _scrollToBottom();
            break;
          case 'done':
            setState(() {
              if (evt.sessionId != null && evt.sessionId!.isNotEmpty) {
                _sessionId = evt.sessionId!;
              }
              if (evt.answer != null && evt.answer!.isNotEmpty) {
                _messages[assistantIdx].content = evt.answer!;
              }
              if (evt.sources.isNotEmpty) {
                _messages[assistantIdx].sources = evt.sources;
              }
              if (evt.messageId != null) {
                _messages[assistantIdx].id = evt.messageId;
              }
              _messages[assistantIdx].streaming = false;
            });
            break;
          case 'error':
            throw ApiException(evt.message ?? '流式回答失败');
        }
      }
      if (mounted) {
        setState(() => _messages[assistantIdx].streaming = false);
      }
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _messages[assistantIdx].streaming = false;
        if (_messages[assistantIdx].content.isEmpty) {
          _messages[assistantIdx].content = '回答失败：${e.message}';
        }
      });
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _messages[assistantIdx].streaming = false;
        if (_messages[assistantIdx].content.isEmpty) {
          _messages[assistantIdx].content = '回答失败：$e';
        }
      });
    } finally {
      if (mounted) setState(() => _sending = false);
      _scrollToBottom();
    }
  }

  void _cancel() {
    setState(() {
      _cancelled = true;
      _sending = false;
      if (_messages.isNotEmpty && _messages.last.streaming) {
        _messages.last.streaming = false;
        if (_messages.last.content.isEmpty) {
          _messages.last.content = '（已取消）';
        }
      }
    });
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scroll.hasClients) return;
      _scroll.animateTo(
        _scroll.position.maxScrollExtent + 80,
        duration: const Duration(milliseconds: 250),
        curve: Curves.easeOut,
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    final kbLabel = state.kbName(_kbId) ?? '选择知识库';
    final dirLabel = state.directoryName(_dirId);

    return Scaffold(
      appBar: AppBar(
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text('对话检索'),
            Text(
              dirLabel == null ? kbLabel : '$kbLabel · $dirLabel',
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w400,
                color: AppTheme.textSecondary,
              ),
            ),
          ],
        ),
        actions: [
          IconButton(
            tooltip: '选择范围',
            onPressed: _pickScope,
            icon: const Icon(Icons.filter_list),
          ),
          IconButton(
            tooltip: '新建会话',
            onPressed: _sending ? null : _newSession,
            icon: const Icon(Icons.add_comment_outlined),
          ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: _messages.isEmpty
                ? const Center(
                    child: Padding(
                      padding: EdgeInsets.all(32),
                      child: Text(
                        '向知识库提问\n答案会附带来源引用',
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          fontSize: 16,
                          color: AppTheme.textSecondary,
                          height: 1.5,
                        ),
                      ),
                    ),
                  )
                : ListView.builder(
                    controller: _scroll,
                    padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
                    itemCount: _messages.length,
                    itemBuilder: (context, index) =>
                        MessageBubble(message: _messages[index]),
                  ),
          ),
          SafeArea(
            top: false,
            child: Container(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
              decoration: const BoxDecoration(
                color: AppTheme.card,
                border: Border(top: BorderSide(color: AppTheme.border)),
              ),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Expanded(
                    child: TextField(
                      controller: _input,
                      minLines: 1,
                      maxLines: 5,
                      textInputAction: TextInputAction.send,
                      onSubmitted: (_) => _send(),
                      enabled: !_sending,
                      decoration: const InputDecoration(
                        hintText: '输入问题…',
                        contentPadding: EdgeInsets.symmetric(
                          horizontal: 14,
                          vertical: 12,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  if (_sending)
                    IconButton.filled(
                      tooltip: '停止生成',
                      onPressed: _cancel,
                      style: IconButton.styleFrom(
                        backgroundColor: AppTheme.danger,
                        foregroundColor: Colors.white,
                        minimumSize: const Size(48, 48),
                      ),
                      icon: const Icon(Icons.stop),
                    )
                  else
                    IconButton.filled(
                      tooltip: '发送问题',
                      onPressed: _send,
                      style: IconButton.styleFrom(
                        backgroundColor: AppTheme.accent,
                        foregroundColor: Colors.white,
                        minimumSize: const Size(48, 48),
                      ),
                      icon: const Icon(Icons.send),
                    ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
