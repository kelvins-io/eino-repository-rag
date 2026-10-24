import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_exception.dart';
import '../services/app_state.dart';
import '../theme/app_theme.dart';
import '../widgets/kb_picker.dart';

class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key});

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  late final TextEditingController _server;
  bool _probing = false;
  String? _probeMsg;
  bool _probeOk = false;

  @override
  void initState() {
    super.initState();
    _server = TextEditingController(text: context.read<AppState>().baseUrl);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final s = context.read<AppState>();
      if (s.isLoggedIn && s.knowledgeBases.isEmpty) {
        s.refreshKnowledgeBases().catchError((_) {});
      }
    });
  }

  @override
  void dispose() {
    _server.dispose();
    super.dispose();
  }

  Future<void> _saveAndProbe() async {
    final state = context.read<AppState>();
    final url = _server.text.trim();
    if (url.isEmpty) {
      setState(() {
        _probeOk = false;
        _probeMsg = '请填写服务端地址';
      });
      return;
    }
    setState(() {
      _probing = true;
      _probeMsg = null;
    });
    try {
      await state.setBaseUrl(url);
      final health = await state.probeHealth();
      setState(() {
        _probeOk = true;
        _probeMsg = '连接成功：${health['status'] ?? 'ok'}';
      });
    } on ApiException catch (e) {
      setState(() {
        _probeOk = false;
        _probeMsg = e.message;
      });
    } catch (e) {
      setState(() {
        _probeOk = false;
        _probeMsg = e.toString();
      });
    } finally {
      if (mounted) setState(() => _probing = false);
    }
  }

  Future<void> _onKbChanged(int? id) async {
    final state = context.read<AppState>();
    try {
      await state.setDefaultKb(id);
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();

    return Scaffold(
      appBar: AppBar(title: const Text('设置')),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
        children: [
          _SectionCard(
            title: '服务端',
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                TextField(
                  controller: _server,
                  keyboardType: TextInputType.url,
                  decoration: const InputDecoration(
                    labelText: '服务端地址',
                    hintText: 'http://192.168.1.10:8080',
                    helperText: 'Android 模拟器可用 10.0.2.2；真机请用局域网 IP',
                    helperMaxLines: 2,
                  ),
                ),
                const SizedBox(height: 12),
                OutlinedButton(
                  onPressed: _probing ? null : _saveAndProbe,
                  child: _probing
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Text('保存并探测连接'),
                ),
                if (_probeMsg != null) ...[
                  const SizedBox(height: 12),
                  Text(
                    _probeMsg!,
                    style: TextStyle(
                      color: _probeOk ? AppTheme.success : AppTheme.danger,
                      fontSize: 14,
                    ),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(height: 16),
          _SectionCard(
            title: '账号',
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  state.user == null
                      ? '未登录'
                      : '${state.user!.displayName.isNotEmpty ? state.user!.displayName : state.user!.username}'
                          '（${state.tenant?.code ?? '-'}）',
                  style: const TextStyle(fontSize: 16),
                ),
                if (state.user != null) ...[
                  const SizedBox(height: 4),
                  Text(
                    '@${state.user!.username}',
                    style: const TextStyle(
                      fontSize: 14,
                      color: AppTheme.textSecondary,
                    ),
                  ),
                  const SizedBox(height: 16),
                  OutlinedButton(
                    onPressed: () async {
                      await state.logout();
                    },
                    child: const Text('退出登录'),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(height: 16),
          _SectionCard(
            title: '默认知识库与目录',
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Text(
                  '对话与扫描录入将默认使用以下归属',
                  style: TextStyle(fontSize: 14, color: AppTheme.textSecondary),
                ),
                const SizedBox(height: 12),
                KnowledgeBaseSelector(
                  items: state.knowledgeBases,
                  value: state.defaultKbId,
                  onChanged: _onKbChanged,
                  enabled: !state.loadingKbs,
                ),
                const SizedBox(height: 12),
                DirectorySelector(
                  tree: state.directories,
                  value: state.defaultDirectoryId,
                  onChanged: (id) => state.setDefaultDirectory(id),
                  enabled: state.defaultKbId != null && !state.loadingKbs,
                ),
                const SizedBox(height: 12),
                TextButton(
                  onPressed: state.loadingKbs
                      ? null
                      : () async {
                          try {
                            await state.refreshKnowledgeBases();
                            if (!context.mounted) return;
                            ScaffoldMessenger.of(context).showSnackBar(
                              const SnackBar(content: Text('知识库列表已刷新')),
                            );
                          } on ApiException catch (e) {
                            if (!context.mounted) return;
                            ScaffoldMessenger.of(context).showSnackBar(
                              SnackBar(content: Text(e.message)),
                            );
                          }
                        },
                  child: state.loadingKbs
                      ? const Text('刷新中…')
                      : const Text('刷新知识库列表'),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _SectionCard extends StatelessWidget {
  const _SectionCard({required this.title, required this.child});

  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppTheme.card,
        borderRadius: BorderRadius.circular(AppTheme.radius),
        border: Border.all(color: AppTheme.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(title, style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 12),
          child,
        ],
      ),
    );
  }
}
