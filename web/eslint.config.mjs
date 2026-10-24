import js from '@eslint/js'
import vue from 'eslint-plugin-vue'
import i18n from '@intlify/eslint-plugin-vue-i18n'
import ts from 'typescript-eslint'
import globals from 'globals'

export default ts.config(
  { ignores: ['dist/**', 'node_modules/**'] },
  js.configs.recommended,
  ...ts.configs.recommended,
  ...vue.configs['flat/recommended'],
  ...i18n.configs['flat/recommended'],
  {
    languageOptions: {
      globals: { ...globals.browser, ...globals.node },
    },
    settings: {
      'vue-i18n': {
        localeDir: './src/i18n/*.json',
        messageSyntaxVersion: '^11.0.0',
      },
    },
    rules: {
      'vue/multi-word-component-names': ['error', { ignores: ['App'] }],
      'vue/html-self-closing': 'off',
      'vue/max-attributes-per-line': 'off',
      'vue/html-closing-bracket-newline': 'off',
      'vue/singleline-html-element-content-newline': 'off',
      'vue/no-restricted-syntax': [
        'error',
        {
          selector:
            "VAttribute[directive=true][key.name.name='bind'][key.argument.name=/^(title|label|placeholder|aria-label|aria-description|alt)$/] > VExpressionContainer > :matches(Literal[value=/\\S/], TemplateLiteral[expressions.length=0])",
          message: 'Translate user-visible attribute literals with t().',
        },
      ],
      '@intlify/vue-i18n/no-raw-text': [
        'error',
        {
          attributes: {
            '/.+/': [
              'title',
              'label',
              'placeholder',
              'aria-label',
              'aria-description',
              'alt',
            ],
          },
        },
      ],
      '@intlify/vue-i18n/no-missing-keys': 'error',
      '@intlify/vue-i18n/no-dynamic-keys': 'error',
      '@intlify/vue-i18n/no-unused-keys': 'off',
    },
  },
  {
    files: ['**/*.vue'],
    languageOptions: { parserOptions: { parser: ts.parser } },
  },
)
