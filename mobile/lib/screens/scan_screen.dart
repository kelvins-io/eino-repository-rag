import 'dart:io';

import 'package:cunning_document_scanner/cunning_document_scanner.dart';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:path/path.dart' as p;
import 'package:provider/provider.dart';

import '../api/api_exception.dart';
import '../api/models.dart';
import '../services/app_state.dart';
import '../theme/app_theme.dart';
import '../widgets/kb_picker.dart';

class ScanScreen extends StatefulWidget {
  const ScanScreen({super.key});

  @override
  State<ScanScreen> createState() => _ScanScreenState();
}

class _ScanScreenState extends State<ScanScreen> {
  final _titleCtrl = TextEditingController();
  final _picker = ImagePicker();
  final List<String> _paths = [];
  int? _kbId;
  int? _dirId;
  bool _uploading = false;
  ImportResult? _lastResult;

  @override
  void initState() {
    super.initState();
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
    });
  }

  @override
  void dispose() {
    _titleCtrl.dispose();
    super.dispose();
  }

  Future<void> _onKbChanged(int? id) async {
    setState(() {
      _kbId = id;
      _dirId = null;
    });
    if (id != null) {
      try {
        await context.read<AppState>().refreshDirectories(id);
      } on ApiException catch (e) {
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
  }

  Future<void> _scanDocuments() async {
    try {
      final pictures = await CunningDocumentScanner.getPictures(
        noOfPages: 20,
        isGalleryImportAllowed: true,
      );
      if (pictures == null || pictures.isEmpty) return;
      setState(() {
        _paths.addAll(pictures);
        _lastResult = null;
      });
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('扫描失败：$e')),
      );
    }
  }

  Future<void> _pickCamera() async {
    try {
      final file = await _picker.pickImage(
        source: ImageSource.camera,
        imageQuality: 90,
      );
      if (file == null) return;
      setState(() {
        _paths.add(file.path);
        _lastResult = null;
      });
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('拍照失败：$e')),
      );
    }
  }

  Future<void> _pickGallery() async {
    try {
      final files = await _picker.pickMultiImage(imageQuality: 90);
      if (files.isEmpty) return;
      setState(() {
        _paths.addAll(files.map((e) => e.path));
        _lastResult = null;
      });
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('选择相册失败：$e')),
      );
    }
  }

  Future<void> _upload() async {
    if (_kbId == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('请先选择知识库')),
      );
      return;
    }
    if (_paths.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('请先扫描或添加图片')),
      );
      return;
    }

    final appState = context.read<AppState>();
    final messenger = ScaffoldMessenger.of(context);
    final title = _titleCtrl.text.trim();
    final kbId = _kbId!;
    final dirId = _dirId;
    final paths = List<String>.from(_paths);

    setState(() {
      _uploading = true;
      _lastResult = null;
    });

    try {
      final files = <MultipartFile>[];
      for (var i = 0; i < paths.length; i++) {
        final path = paths[i];
        final name = p.basename(path);
        final ext = p.extension(name).toLowerCase();
        final filename = name.isEmpty || !name.contains('.')
            ? 'scan_${i + 1}.jpg'
            : name;
        final contentType = ext == '.png'
            ? DioMediaType('image', 'png')
            : DioMediaType('image', 'jpeg');
        files.add(
          await MultipartFile.fromFile(
            path,
            filename: filename,
            contentType: contentType,
          ),
        );
      }

      final result = await appState.uploadFiles(
            knowledgeBaseId: kbId,
            directoryId: dirId,
            title: title.isEmpty ? null : title,
            files: files,
          );

      if (!mounted) return;
      setState(() {
        _lastResult = result;
        _paths.clear();
      });
      messenger.showSnackBar(
        SnackBar(
          content: Text(
            result.message.isNotEmpty
                ? result.message
                : '导入完成：新增 ${result.imported}，去重 ${result.duplicated}',
          ),
        ),
      );
    } on ApiException catch (e) {
      messenger.showSnackBar(SnackBar(content: Text(e.message)));
    } catch (e) {
      messenger.showSnackBar(SnackBar(content: Text('上传失败：$e')));
    } finally {
      if (mounted) setState(() => _uploading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();

    return Scaffold(
      appBar: AppBar(title: const Text('录入文档')),
      body: Column(
        children: [
          Expanded(
            child: ListView(
              padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
              children: [
                Container(
                  padding: const EdgeInsets.all(16),
                  decoration: BoxDecoration(
                    color: AppTheme.card,
                    borderRadius: BorderRadius.circular(AppTheme.radius),
                    border: Border.all(color: AppTheme.border),
                  ),
                  child: Column(
                    children: [
                      KnowledgeBaseSelector(
                        items: state.knowledgeBases,
                        value: _kbId,
                        onChanged: _onKbChanged,
                      ),
                      const SizedBox(height: 12),
                      DirectorySelector(
                        tree: state.directories,
                        value: _dirId,
                        onChanged: (id) => setState(() => _dirId = id),
                        enabled: _kbId != null,
                      ),
                      const SizedBox(height: 12),
                      TextField(
                        controller: _titleCtrl,
                        decoration: const InputDecoration(
                          labelText: '文档标题（可选）',
                          hintText: '不填则使用文件名',
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 16),
                Row(
                  children: [
                    Expanded(
                      child: OutlinedButton.icon(
                        onPressed: _uploading ? null : _scanDocuments,
                        icon: const Icon(Icons.document_scanner_outlined),
                        label: const Text('扫描文档'),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: OutlinedButton.icon(
                        onPressed: _uploading ? null : _pickCamera,
                        icon: const Icon(Icons.photo_camera_outlined),
                        label: const Text('拍照'),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                OutlinedButton.icon(
                  onPressed: _uploading ? null : _pickGallery,
                  icon: const Icon(Icons.photo_library_outlined),
                  label: const Text('从相册选择'),
                ),
                const SizedBox(height: 16),
                Text(
                  _paths.isEmpty ? '尚未添加页面' : '待上传 ${_paths.length} 页',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 8),
                if (_paths.isEmpty)
                  Container(
                    height: 140,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: AppTheme.card,
                      borderRadius: BorderRadius.circular(AppTheme.radius),
                      border: Border.all(color: AppTheme.border),
                    ),
                    child: const Text(
                      '扫描纸质件或从相册导入图片\n服务端将自动 OCR 建索引',
                      textAlign: TextAlign.center,
                      style: TextStyle(color: AppTheme.textSecondary, fontSize: 14),
                    ),
                  )
                else
                  SizedBox(
                    height: 140,
                    child: ListView.separated(
                      scrollDirection: Axis.horizontal,
                      itemCount: _paths.length,
                      separatorBuilder: (_, __) => const SizedBox(width: 8),
                      itemBuilder: (context, index) {
                        final path = _paths[index];
                        return Stack(
                          children: [
                            ClipRRect(
                              borderRadius:
                                  BorderRadius.circular(AppTheme.radiusSm),
                              child: Image.file(
                                File(path),
                                width: 110,
                                height: 140,
                                fit: BoxFit.cover,
                              ),
                            ),
                            Positioned(
                              top: 4,
                              right: 4,
                              child: Material(
                                color: Colors.black54,
                                shape: const CircleBorder(),
                                child: InkWell(
                                  customBorder: const CircleBorder(),
                                  onTap: _uploading
                                      ? null
                                      : () => setState(() => _paths.removeAt(index)),
                                  child: const SizedBox(
                                    width: 32,
                                    height: 32,
                                    child: Icon(
                                      Icons.close,
                                      size: 18,
                                      color: Colors.white,
                                    ),
                                  ),
                                ),
                              ),
                            ),
                            Positioned(
                              left: 6,
                              bottom: 6,
                              child: Container(
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 6,
                                  vertical: 2,
                                ),
                                decoration: BoxDecoration(
                                  color: Colors.black54,
                                  borderRadius: BorderRadius.circular(4),
                                ),
                                child: Text(
                                  '${index + 1}',
                                  style: const TextStyle(
                                    color: Colors.white,
                                    fontSize: 12,
                                  ),
                                ),
                              ),
                            ),
                          ],
                        );
                      },
                    ),
                  ),
                if (_lastResult != null) ...[
                  const SizedBox(height: 20),
                  Text('上次导入结果', style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 8),
                  for (final item in _lastResult!.items)
                    Container(
                      margin: const EdgeInsets.only(bottom: 8),
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        color: AppTheme.card,
                        borderRadius: BorderRadius.circular(AppTheme.radiusSm),
                        border: Border.all(color: AppTheme.border),
                      ),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            item.fileName,
                            style: const TextStyle(
                              fontWeight: FontWeight.w600,
                              fontSize: 15,
                            ),
                          ),
                          const SizedBox(height: 4),
                          Text(
                            item.duplicated
                                ? '已去重（内容相同）'
                                : '已入队索引 · ${item.document?.status ?? 'pending'}',
                            style: TextStyle(
                              fontSize: 13,
                              color: item.duplicated
                                  ? AppTheme.textSecondary
                                  : AppTheme.success,
                            ),
                          ),
                          if (item.message.isNotEmpty) ...[
                            const SizedBox(height: 4),
                            Text(
                              item.message,
                              style: const TextStyle(
                                fontSize: 12,
                                color: AppTheme.textSecondary,
                              ),
                            ),
                          ],
                        ],
                      ),
                    ),
                ],
              ],
            ),
          ),
          SafeArea(
            top: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
              child: FilledButton(
                onPressed: _uploading ? null : _upload,
                child: _uploading
                    ? const SizedBox(
                        width: 22,
                        height: 22,
                        child: CircularProgressIndicator(
                          strokeWidth: 2.5,
                          color: Colors.white,
                        ),
                      )
                    : Text('上传 ${_paths.isEmpty ? '文档' : '${_paths.length} 页'}'),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
