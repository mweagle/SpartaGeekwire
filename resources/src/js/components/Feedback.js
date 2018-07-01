import React, { Component } from 'react';
import Card from 'grommet/components/Card';
import Heading from 'grommet/components/Heading';
import Distribution from 'grommet/components/Distribution';

export default class Feedback extends Component {
  render() {
    if (!this.props.feedback) {
      return null;
    }
    var sentimentState = (this.props.feedback &&
      this.props.feedback.sentiment &&
      this.props.feedback.sentiment.SentimentScore) ?
      this.props.feedback.sentiment.SentimentScore : {};

    // Add the colors...
    var sentimentEntry = function(label, colorCode) {
      var value = sentimentState[label] || 0.0;
      var normalizedValue = Math.round((100 * value));
      return {"label": label,
        "value": normalizedValue,
        "colorIndex": colorCode
      };
    };
    var series = [];
    series.push(sentimentEntry("Mixed", "unknown"));
    series.push(sentimentEntry("Negative", "critical"));
    series.push(sentimentEntry("Neutral", "warning"));
    series.push(sentimentEntry("Positive", "ok"));
    return (
      <Card
        contentPad="large"
        heading={
          <Heading strong={false}>
            Sentiment
          </Heading>
        }
        size="large">
        <Distribution series={series} />
      </Card>
    );
  }
};
