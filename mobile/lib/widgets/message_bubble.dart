import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../api/models.dart';
import '../theme/app_theme.dart';

class MessageBubble extends StatelessWidget {
  const MessageBubble({super.key, required this.message});

  final ChatMessage message;

  bool get _isUser => message.role == 'user';

  @override
  Widget build(BuildContext context) {
    final align = _isUser ? Alignment.centerRight : Alignment.centerLeft;
    final bg = _isUser ? AppTheme.accent : AppTheme.card;
    final fg = _isUser ? Colors.white : AppTheme.textPrimary;
    final border = _isUser ? null : Border.all(color: AppTheme.border);

    return Align(
      alignment: align,
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxWidth: MediaQuery.of(context).size.width * 0.86,
        ),
        child: Column(
          crossAxisAlignment:
              _isUser ? CrossAxisAlignment.end : CrossAxisAlignment.start,
          children: [
            Container(
              margin: const EdgeInsets.symmetric(vertical: 6),
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
              decoration: BoxDecoration(
                color: bg,
                borderRadius: BorderRadius.only(
                  topLeft: const Radius.circular(AppTheme.radius),
                  topRight: const Radius.circular(AppTheme.radius),
                  bottomLeft: Radius.circular(_isUser ? AppTheme.radius : 4),
                  bottomRight: Radius.circular(_isUser ? 4 : AppTheme.radius),
                ),
                border: border,
              ),
              child: SelectableText(
                message.content.isEmpty && message.streaming
                    ? '…'
                    : message.content,
                style: TextStyle(color: fg, fontSize: 16, height: 1.45),
              ),
            ),
            if (!_isUser && message.sources.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: SourceList(sources: message.sources),
              ),
          ],
        ),
      ),
    );
  }
}

class SourceList extends StatelessWidget {
  const SourceList({super.key, required this.sources});

  final List<SourceDocument> sources;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Padding(
          padding: EdgeInsets.only(left: 4, bottom: 6),
          child: Text(
            '引用来源',
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: AppTheme.textSecondary,
            ),
          ),
        ),
        for (var i = 0; i < sources.length; i++)
          SourceCard(index: i + 1, source: sources[i]),
      ],
    );
  }
}

class SourceCard extends StatelessWidget {
  const SourceCard({super.key, required this.index, required this.source});

  final int index;
  final SourceDocument source;

  @override
  Widget build(BuildContext context) {
    final title = source.title.isNotEmpty ? source.title : '片段 $index';
    final meta = <String>[
      if (source.section.isNotEmpty) source.section,
      if (source.page > 0) 'p.${source.page}',
    ].join(' · ');

    return InkWell(
      borderRadius: BorderRadius.circular(AppTheme.radiusSm),
      onTap: () async {
        await Clipboard.setData(ClipboardData(text: source.content));
        if (context.mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('已复制来源内容')),
          );
        }
      },
      child: Container(
        width: double.infinity,
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
              '[$index] $title',
              style: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w600,
                color: AppTheme.textPrimary,
              ),
            ),
            if (meta.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                meta,
                style: const TextStyle(fontSize: 12, color: AppTheme.textSecondary),
              ),
            ],
            if (source.source.isNotEmpty) ...[
              const SizedBox(height: 4),
              GestureDetector(
                onTap: () async {
                  await Clipboard.setData(ClipboardData(text: source.source));
                  if (context.mounted) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(content: Text('已复制文档来源链接')),
                    );
                  }
                },
                child: Text(
                  '来源：${source.source}',
                  style: const TextStyle(
                    fontSize: 12,
                    color: AppTheme.accent,
                    decoration: TextDecoration.underline,
                  ),
                ),
              ),
            ],
            const SizedBox(height: 6),
            Text(
              source.content,
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 13,
                color: AppTheme.textSecondary,
                height: 1.4,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
