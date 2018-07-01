import React, { Component } from 'react';
import Card from 'grommet/components/Card';
import { TagCloud } from "react-tagcloud";
import Heading from 'grommet/components/Heading';

export default class LabelsCloud extends Component {

  render() {
    if (!this.props.consolidatedResponse ||
        !this.props.consolidatedResponse.rekognition ||
        !this.props.consolidatedResponse.rekognition.Labels) {
      return null;
    }
    var data = this.props.consolidatedResponse.rekognition.Labels.map(eachObj => {
      return {
        count: Math.round(eachObj.Confidence),
        value: eachObj.Name
      };
    });
    var colorOptions = {
      hue: "red",
      luminosity: "dark",
      seed: 42
    };

    return (
      <Card
        contentPad="large"
        heading={
          <Heading strong={false}>
            Labels
          </Heading>
        }
        size="large">
        <TagCloud minSize={10}
          maxSize={36}
          shuffle={false}
          disableRandomColor={false}
          tags={data}
          colorOptions={colorOptions}
          onClick={tag => alert(`'${tag.value}' was selected!`)} />
      </Card>
    );
  }
};
