#!/usr/bin/env python3
"""核验人工证据矩阵并确定性计分，不代替业务与过程审查。"""
import argparse
import json
from decimal import Decimal, InvalidOperation
from fractions import Fraction
from pathlib import Path

LIMITS = {key: Decimal(value) for key, value in {
    'A1': '3.00', 'A2': '1.80', 'A3': '1.70', 'A4': '1.50',
    'B1': '0.60', 'B2': '0.80', 'B3': '0.30', 'B4': '0.30',
}.items()}
FACTORS = {'pass': Decimal('1'), 'partial': Decimal('0.5'),
           'fail': Decimal('0'), 'unknown': Decimal('0')}
ZERO = Decimal('0')
CENT = Decimal('0.01')
GATE_CAP = Decimal('7.90')
PASS_SCORE = Decimal('8.00')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def nonempty(value):
    return isinstance(value, str) and bool(value.strip())


def money(value):
    require(isinstance(value, (str, int, float)) and not isinstance(value, bool),
            'max_points 必须为数值或数值字符串')
    try:
        number = Decimal(str(value))
        require(number.is_finite() and ZERO < number <= Decimal('10'),
                'max_points 须为 0 到 10 之间的有限正数')
        require(number == number.quantize(CENT), 'max_points 最多两位小数')
        return number
    except InvalidOperation as exc:
        raise ValueError('max_points 不是有效数值') from exc


def validate_item(item, seen):
    require(isinstance(item, dict), 'check 必须为对象')
    key = item.get('id')
    require(nonempty(key) and key not in seen, 'check id 必须非空且唯一')
    seen.add(key)
    require(item.get('category') in LIMITS, f'{key}: 无效 category')
    status = item.get('status')
    require(status in {*FACTORS, 'na'}, f'{key}: 无效 status')
    require(isinstance(item.get('mandatory'), bool), f'{key}: mandatory 必须是布尔值')
    require(nonempty(item.get('reason')), f'{key}: 缺少具体 reason')
    evidence = item.get('evidence')
    require(isinstance(evidence, list) and all(nonempty(e) for e in evidence),
            f'{key}: evidence 必须是证据编号字符串列表')
    if status == 'na':
        require(nonempty(item.get('condition')), f'{key}: na 必须说明题面条件')
        require(bool(evidence), f'{key}: na 必须有条件证据')
    else:
        require(bool(evidence), f'{key}: 必须有核查或缺口证据')
    return {**item, '_weight': money(item.get('max_points'))}


def validate_input(data):
    require(isinstance(data, dict), '输入必须为 JSON 对象')
    for key in ('student', 'question', 'revision'):
        require(nonempty(data.get(key)), f'缺少 {key}')
    checks = data.get('checks')
    require(isinstance(checks, list) and bool(checks), 'checks 必须为非空列表')
    seen = set()
    items = [validate_item(item, seen) for item in checks]
    for category, limit in LIMITS.items():
        category_items = [i for i in items if i['category'] == category]
        require(sum((i['_weight'] for i in category_items), ZERO) == limit,
                f'{category}: 原子项满分之和须为 {limit}')
        require(any(i['status'] != 'na' for i in category_items),
                f'{category}: 不能全部不适用')
    return items


def displayed(value):
    value = Fraction(value)
    cents = value.numerator * 100 // value.denominator
    return f'{cents // 100}.{cents % 100:02d}'


def category_result(category, items):
    applicable = [i for i in items if i['category'] == category and i['status'] != 'na']
    weight = sum((i['_weight'] for i in applicable), ZERO)
    normalized = []
    for item in applicable:
        maximum = Fraction(LIMITS[category]) * Fraction(item['_weight']) / Fraction(weight)
        points = maximum * Fraction(FACTORS[item['status']])
        normalized.append({'id': item['id'], 'status': item['status'],
                           'mandatory': item['mandatory'], 'factor': str(FACTORS[item['status']]),
                           'normalized_max_exact': str(maximum), 'points_exact': str(points),
                           'loss_exact': str(maximum - points),
                           'evidence': item['evidence'], 'reason': item['reason']})
    points = sum((Fraction(i['points_exact']) for i in normalized), Fraction(0))
    return {'category': category, 'max': str(LIMITS[category]),
            'applicable_weight': str(weight), 'score': displayed(points),
            'score_exact': str(points), 'checks': normalized}


def blocker_result(item):
    unknown = item['status'] == 'unknown'
    return {'id': item['id'], 'category': item['category'], 'status': item['status'],
            'label': '待补证，尚不能确认合规' if unknown else '必做要求未完全通过',
            'reason': item['reason'], 'evidence': item['evidence']}


def score_audit(data):
    items = validate_input(data)
    categories = [category_result(key, items) for key in LIMITS]
    completion = sum((Fraction(i['score_exact']) for i in categories[:4]), Fraction(0))
    process = sum((Fraction(i['score_exact']) for i in categories[4:]), Fraction(0))
    raw = completion + process
    blockers = [blocker_result(i) for i in items
                if i['mandatory'] and i['status'] not in ('pass', 'na')]
    final = min(raw, Fraction(GATE_CAP)) if blockers else raw
    shown_final = displayed(final)
    return {'student': data['student'], 'question': data['question'],
            'revision': data['revision'], 'completion_score': displayed(completion),
            'process_score': displayed(process), 'raw_score': displayed(raw),
            'raw_score_exact': str(raw), 'gate_cap': str(GATE_CAP) if blockers else None,
            'gate_adjustment_exact': str(raw - final), 'final_score': shown_final,
            'verdict': '合格' if Decimal(shown_final) >= PASS_SCORE else '不合格',
            'evidence_provisional': any(i['status'] == 'unknown' for i in items),
            'counts': {status: sum(i['status'] == status for i in items)
                       for status in (*FACTORS, 'na')},
            'blockers': blockers, 'categories': categories}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    try:
        require(args.input.resolve() != args.output.resolve(), '输出不得覆盖评分输入')
        data = json.loads(args.input.read_text(encoding='utf-8-sig'))
        result = score_audit(data)
    except (ValueError, OSError) as exc:
        parser.exit(2, f'计分失败：{exc}\n')
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n',
                           encoding='utf-8')
    print(f"{result['final_score']}/10 {result['verdict']}；原始分 {result['raw_score']}")


if __name__ == '__main__':
    main()
