import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_exception.dart';
import '../services/app_state.dart';
import '../theme/app_theme.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _tenant;
  late final TextEditingController _username;
  late final TextEditingController _password;
  late final TextEditingController _server;
  bool _obscure = true;
  bool _busy = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    final state = context.read<AppState>();
    _tenant = TextEditingController(text: state.tenantPrefill);
    _username = TextEditingController();
    _password = TextEditingController();
    _server = TextEditingController(text: state.baseUrl);
  }

  @override
  void dispose() {
    _tenant.dispose();
    _username.dispose();
    _password.dispose();
    _server.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    final state = context.read<AppState>();
    try {
      await state.setBaseUrl(_server.text.trim());
      await state.login(
        tenantId: _tenant.text.trim(),
        username: _username.text.trim(),
        password: _password.text,
      );
    } on ApiException catch (e) {
      setState(() => _error = e.message);
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 32),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Form(
                key: _formKey,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      'Eino 知识库',
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 8),
                    const Text(
                      '登录后即可扫描录入文档并对话检索',
                      style: TextStyle(
                        fontSize: 16,
                        color: AppTheme.textSecondary,
                      ),
                    ),
                    const SizedBox(height: 32),
                    TextFormField(
                      controller: _server,
                      keyboardType: TextInputType.url,
                      textInputAction: TextInputAction.next,
                      decoration: const InputDecoration(
                        labelText: '服务端地址',
                        hintText: 'http://192.168.1.10:8080',
                      ),
                      validator: (v) {
                        if (v == null || v.trim().isEmpty) {
                          return '请填写服务端地址';
                        }
                        final u = Uri.tryParse(v.trim());
                        if (u == null || !u.hasScheme || u.host.isEmpty) {
                          return '地址格式不正确，例如 http://192.168.1.10:8080';
                        }
                        return null;
                      },
                    ),
                    const SizedBox(height: 16),
                    TextFormField(
                      controller: _tenant,
                      textInputAction: TextInputAction.next,
                      decoration: const InputDecoration(
                        labelText: '租户 ID',
                        hintText: 'guest',
                      ),
                      validator: (v) =>
                          (v == null || v.trim().isEmpty) ? '请填写租户 ID' : null,
                    ),
                    const SizedBox(height: 16),
                    TextFormField(
                      controller: _username,
                      textInputAction: TextInputAction.next,
                      decoration: const InputDecoration(labelText: '用户名'),
                      validator: (v) =>
                          (v == null || v.trim().isEmpty) ? '请填写用户名' : null,
                    ),
                    const SizedBox(height: 16),
                    TextFormField(
                      controller: _password,
                      obscureText: _obscure,
                      textInputAction: TextInputAction.done,
                      onFieldSubmitted: (_) => _submit(),
                      decoration: InputDecoration(
                        labelText: '密码',
                        suffixIcon: IconButton(
                          tooltip: _obscure ? '显示密码' : '隐藏密码',
                          onPressed: () =>
                              setState(() => _obscure = !_obscure),
                          icon: Icon(
                            _obscure
                                ? Icons.visibility_outlined
                                : Icons.visibility_off_outlined,
                          ),
                        ),
                      ),
                      validator: (v) =>
                          (v == null || v.isEmpty) ? '请填写密码' : null,
                    ),
                    if (_error != null) ...[
                      const SizedBox(height: 16),
                      Text(
                        _error!,
                        style: const TextStyle(
                          color: AppTheme.danger,
                          fontSize: 14,
                        ),
                      ),
                    ],
                    const SizedBox(height: 28),
                    FilledButton(
                      onPressed: _busy ? null : _submit,
                      child: _busy
                          ? const SizedBox(
                              width: 22,
                              height: 22,
                              child: CircularProgressIndicator(
                                strokeWidth: 2.5,
                                color: Colors.white,
                              ),
                            )
                          : const Text('登录账号'),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
